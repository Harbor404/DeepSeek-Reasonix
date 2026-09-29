import { checkLeases } from "./accounts";
import type { Env, RemoteCapability } from "./env";
import {
  CLOSE_DEVICE_REPLACED,
  CLOSE_DISCONNECTED_BY_DEVICE,
  CLOSE_IDLE,
  CLOSE_REAUTH_REQUIRED,
  CLOSE_REVOKED,
  EVICTABLE_IDLE_MS,
  HEARTBEAT_MS,
  HEARTBEAT_REQUEST,
  HEARTBEAT_RESPONSE,
  lastActive,
  leaseEnd,
  nextDeadline,
  readLease,
  REVALIDATE_MS,
  SESSION_REVOCATION_TTL_MS,
  UNCHECKED_LIMIT_MS,
  verdictEnd,
  type LeaseEnd,
  type SocketLease,
} from "./lease";
import {
  controllerMessage,
  controllerPresence,
  offeredProtocols,
  parseDeviceMessage,
  relayHello,
  REMOTE_WEBSOCKET_PROTOCOL,
} from "./protocol";

const MAX_MESSAGE_BYTES = 64 * 1024;
const MAX_CONTROLLERS = 4;
const ADMISSION_SKEW_MS = 60 * 1000;
const SESSION_ID = /^[0-9a-f]{64}$/;
const DEVICE_ID = /^[0-9a-f]{64}$/;
const REVOKED_AT_KEY = "revokedAt";
const CHECKED_AT_KEY = "checkedAt";
const CHECKED_OK_AT_KEY = "checkedOkAt";
const REVOKED_SESSIONS_KEY = "revokedSessions";

function messageBytes(message: string): number {
  return new TextEncoder().encode(message).byteLength;
}

function connectionId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
}

function leaseOf(socket: WebSocket): SocketLease | null {
  return readLease(socket.deserializeAttachment());
}

// One room per registered device. A connection is admitted once, at the
// WebSocket upgrade; after that it lives on a lease that slides with its own
// traffic, ends at the signing-in session's re-authentication time, and is
// re-checked against the account service every REVALIDATE_MS.
export class RemoteSession {
  constructor(
    private readonly state: DurableObjectState,
    private readonly env: Env,
  ) {
    state.setWebSocketAutoResponse(new WebSocketRequestResponsePair(HEARTBEAT_REQUEST, HEARTBEAT_RESPONSE));
  }

  async fetch(request: Request): Promise<Response> {
    const path = new URL(request.url).pathname;
    if (path === "/status") {
      const online = this.state.getWebSockets("device").some((socket) =>
        socket.readyState === WebSocket.OPEN && leaseOf(socket) !== null);
      return Response.json({ online });
    }
    if (path === "/revoke" && request.method === "POST") {
      return this.revoke(request);
    }
    if (request.headers.get("upgrade")?.toLowerCase() !== "websocket") {
      return new Response("WebSocket upgrade required", { status: 426 });
    }
    return this.admit(request);
  }

  private async admit(request: Request): Promise<Response> {
    const now = Date.now();
    const role = request.headers.get("x-reasonix-role");
    const userId = Number(request.headers.get("x-reasonix-user-id"));
    const deviceId = request.headers.get("x-reasonix-device-id") ?? "";
    const admittedAt = Number(request.headers.get("x-reasonix-admitted-at"));
    if ((role !== "device" && role !== "controller") || !Number.isSafeInteger(userId) || userId < 1 ||
        !DEVICE_ID.test(deviceId) || !Number.isSafeInteger(admittedAt) || Math.abs(now - admittedAt) > ADMISSION_SKEW_MS) {
      return new Response("Invalid admission", { status: 401 });
    }
    const sessionHeader = request.headers.get("x-reasonix-session");
    const sessionId = sessionHeader && SESSION_ID.test(sessionHeader) ? sessionHeader : null;
    const reauthHeader = request.headers.get("x-reasonix-reauth-at");
    const reauthAt = role === "controller" ? Number(reauthHeader) : null;
    if (reauthAt !== null && (!Number.isSafeInteger(reauthAt) || reauthAt <= now)) {
      return new Response("Sign-in required", { status: 401 });
    }
    const revokedAt = await this.state.storage.get<number>(REVOKED_AT_KEY);
    if (revokedAt !== undefined && admittedAt <= revokedAt) {
      return new Response("Device revoked", { status: 403 });
    }
    const sessionRevokedAt = sessionId ? (await this.revokedSessions(now))[sessionId] : undefined;
    if (sessionRevokedAt !== undefined && admittedAt <= sessionRevokedAt) {
      return new Response("Signed out", { status: 401 });
    }
    const scopes = (request.headers.get("x-reasonix-scopes") ?? "")
      .split(",")
      .filter(Boolean) as RemoteCapability[];

    const devices = this.live("device");
    const controllers = this.live("controller");
    const ownerMismatch = [...devices, ...controllers].some(([, lease]) => lease.userId !== userId);
    if (ownerMismatch) return new Response("Session owner mismatch", { status: 403 });
    if (role === "controller" && controllers.length >= MAX_CONTROLLERS) {
      const quietest = controllers
        .map(([socket, lease]) => ({ socket, lease, at: lastActive(lease, this.state.getWebSocketAutoResponseTimestamp(socket)) }))
        .sort((a, b) => a.at - b.at)[0];
      if (!quietest || now - quietest.at < EVICTABLE_IDLE_MS) {
        return new Response("Too many controllers", { status: 429 });
      }
      this.end(quietest.socket, quietest.lease, { code: CLOSE_IDLE, reason: "Replaced by a newer connection" });
    }

    const pair = new WebSocketPair();
    const client = pair[0];
    const server = pair[1];
    const lease: SocketLease = {
      role,
      userId,
      deviceId,
      scopes,
      connectionId: role === "controller" ? connectionId() : null,
      sessionId: role === "controller" ? sessionId : null,
      admittedAt,
      lastActiveAt: now,
      reauthAt,
    };
    server.serializeAttachment(lease);
    this.state.acceptWebSocket(server, [role]);

    if (role === "device") {
      for (const [oldDevice] of devices) oldDevice.close(CLOSE_DEVICE_REPLACED, "Device reconnected");
      const connectionIds: string[] = [];
      for (const [, peer] of controllers) {
        if (!peer.connectionId) continue;
        connectionIds.push(peer.connectionId);
        server.send(controllerPresence("controller_connected", peer.connectionId, peer.scopes));
      }
      server.send(relayHello(connectionIds, HEARTBEAT_REQUEST, HEARTBEAT_MS));
    } else if (lease.connectionId) {
      for (const [device] of devices) {
        device.send(controllerPresence("controller_connected", lease.connectionId, lease.scopes));
      }
    }
    if ((await this.state.storage.get<number>(CHECKED_AT_KEY)) === undefined) {
      await this.state.storage.put(CHECKED_AT_KEY, now);
      await this.state.storage.put(CHECKED_OK_AT_KEY, now);
    }
    await this.schedule();
    const headers = new Headers();
    if (offeredProtocols(request).includes(REMOTE_WEBSOCKET_PROTOCOL)) {
      headers.set("sec-websocket-protocol", REMOTE_WEBSOCKET_PROTOCOL);
    }
    return new Response(null, { status: 101, webSocket: client, headers });
  }

  private async revoke(request: Request): Promise<Response> {
    let body: { sessionId?: unknown } = {};
    try {
      body = await request.json();
    } catch {
      // An empty body revokes the whole device.
    }
    let closed = 0;
    if (typeof body.sessionId === "string") {
      const revoked = await this.revokedSessions(Date.now());
      revoked[body.sessionId] = Date.now();
      await this.state.storage.put(REVOKED_SESSIONS_KEY, revoked);
      for (const [socket, lease] of this.live("controller")) {
        if (lease.sessionId !== body.sessionId) continue;
        this.end(socket, lease, { code: CLOSE_REAUTH_REQUIRED, reason: "Signed out" });
        closed += 1;
      }
      return Response.json({ closed });
    }
    await this.state.storage.put(REVOKED_AT_KEY, Date.now());
    for (const [socket, lease] of [...this.live("controller"), ...this.live("device")]) {
      this.end(socket, lease, { code: CLOSE_REVOKED, reason: "Access revoked" });
      closed += 1;
    }
    return Response.json({ closed });
  }

  async alarm(): Promise<void> {
    const now = Date.now();
    for (const [socket, lease] of [...this.live("controller"), ...this.live("device")]) {
      const end = leaseEnd(lease, now, this.state.getWebSocketAutoResponseTimestamp(socket));
      if (end) this.end(socket, lease, end);
    }
    const checkedAt = await this.state.storage.get<number>(CHECKED_AT_KEY);
    if (checkedAt === undefined || now - checkedAt >= REVALIDATE_MS) {
      if (await this.revalidate()) {
        await this.state.storage.put(CHECKED_OK_AT_KEY, now);
      } else if (now - ((await this.state.storage.get<number>(CHECKED_OK_AT_KEY)) ?? now) >= UNCHECKED_LIMIT_MS) {
        for (const [socket, lease] of [...this.live("controller"), ...this.live("device")]) {
          this.end(socket, lease, { code: CLOSE_IDLE, reason: "Access could not be re-checked" });
        }
      }
      await this.state.storage.put(CHECKED_AT_KEY, now);
    }
    await this.schedule();
  }

  async webSocketMessage(socket: WebSocket, message: string | ArrayBuffer): Promise<void> {
    if (typeof message !== "string") {
      socket.close(1003, "Binary messages are not supported");
      return;
    }
    if (messageBytes(message) > MAX_MESSAGE_BYTES) {
      socket.close(1009, "Message too large");
      return;
    }
    const sender = leaseOf(socket);
    if (!sender) {
      socket.close(1008, "Missing session identity");
      return;
    }
    sender.lastActiveAt = Date.now();
    socket.serializeAttachment(sender);
    if (sender.role === "controller") {
      if (!sender.connectionId) {
        socket.close(1008, "Missing connection identity");
        return;
      }
      for (const [device, peer] of this.live("device")) {
        if (peer.userId === sender.userId && device.readyState === WebSocket.OPEN) {
          device.send(controllerMessage(sender.connectionId, sender.scopes, message));
        }
      }
      return;
    }

    const deviceMessage = parseDeviceMessage(message);
    if (!deviceMessage) {
      socket.close(1008, "A directed device message is required");
      return;
    }
    if (deviceMessage.type === "heartbeat") {
      socket.send(HEARTBEAT_RESPONSE);
      return;
    }
    if (deviceMessage.type === "disconnect_controller") {
      for (const [controller, peer] of this.live("controller")) {
        if (peer.userId === sender.userId && peer.connectionId === deviceMessage.connectionId) {
          controller.close(CLOSE_DISCONNECTED_BY_DEVICE, "Disconnected by device");
        }
      }
      return;
    }
    for (const [controller, peer] of this.live("controller")) {
      if (peer.userId === sender.userId && peer.connectionId === deviceMessage.to && controller.readyState === WebSocket.OPEN) {
        controller.send(deviceMessage.payload);
      }
    }
  }

  async webSocketClose(socket: WebSocket, code: number, reason: string, wasClean: boolean): Promise<void> {
    const lease = leaseOf(socket);
    if (lease?.role === "controller" && lease.connectionId) this.announceGone(lease);
    socket.close(code, wasClean ? reason : "Connection closed");
  }

  private live(role: SocketLease["role"]): Array<[WebSocket, SocketLease]> {
    const sockets: Array<[WebSocket, SocketLease]> = [];
    for (const socket of this.state.getWebSockets(role)) {
      const lease = leaseOf(socket);
      if (lease && socket.readyState === WebSocket.OPEN) sockets.push([socket, lease]);
    }
    return sockets;
  }

  private end(socket: WebSocket, lease: SocketLease, end: LeaseEnd): void {
    try {
      socket.close(end.code, end.reason);
    } catch {
      // Already closing; the peer notice below still has to go out.
    }
    if (lease.role === "controller" && lease.connectionId) this.announceGone(lease);
  }

  private announceGone(lease: SocketLease): void {
    if (!lease.connectionId) return;
    for (const [device] of this.live("device")) {
      device.send(controllerPresence("controller_disconnected", lease.connectionId, lease.scopes));
    }
  }

  // Whether the account service answered; a failed check keeps connections
  // open until UNCHECKED_LIMIT_MS passes without a good one.
  private async revalidate(): Promise<boolean> {
    const sockets = [...this.live("controller"), ...this.live("device")];
    if (sockets.length === 0) return true;
    const verdicts = await checkLeases(this.env, sockets.map(([, lease]) => ({
      userId: lease.userId,
      deviceId: lease.deviceId,
      ...(lease.sessionId ? { sessionId: lease.sessionId } : {}),
    })));
    if (!verdicts || verdicts.length !== sockets.length) return false;
    sockets.forEach(([socket, lease], index) => {
      const end = verdictEnd(verdicts[index] ?? "active");
      if (end) this.end(socket, lease, end);
    });
    return true;
  }

  private async revokedSessions(now: number): Promise<Record<string, number>> {
    const stored = (await this.state.storage.get<Record<string, number>>(REVOKED_SESSIONS_KEY)) ?? {};
    const fresh: Record<string, number> = {};
    for (const [id, at] of Object.entries(stored)) {
      if (now - at < SESSION_REVOCATION_TTL_MS) fresh[id] = at;
    }
    return fresh;
  }

  private async schedule(): Promise<void> {
    const sockets = [...this.live("controller"), ...this.live("device")];
    if (sockets.length === 0) {
      await this.state.storage.deleteAlarm();
      return;
    }
    const checkedAt = (await this.state.storage.get<number>(CHECKED_AT_KEY)) ?? Date.now();
    let next = checkedAt + REVALIDATE_MS;
    for (const [socket, lease] of sockets) {
      next = Math.min(next, nextDeadline(lease, this.state.getWebSocketAutoResponseTimestamp(socket)));
    }
    const current = await this.state.storage.getAlarm();
    const at = Math.max(next, Date.now() + 1000);
    if (current === null || current > at || current <= Date.now()) await this.state.storage.setAlarm(at);
  }
}
