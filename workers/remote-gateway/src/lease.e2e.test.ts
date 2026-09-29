import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import worker from "./index";
import { RemoteSession } from "./session";
import type { Env } from "./env";
import {
  CLOSE_IDLE,
  CLOSE_REAUTH_REQUIRED,
  CLOSE_REVOKED,
  CONTROLLER_IDLE_MS,
  REVALIDATE_MS,
} from "./lease";
import {
  FakeRequestResponsePair,
  FakeSocket,
  FakeState,
  FakeWebSocketPair,
  upgradeCapableResponse,
} from "./fake_room";

const MINUTE = 60 * 1000;
const HOUR = 60 * MINUTE;
const REAUTH_MS = 24 * HOUR;
const OLD_CONTROLLER_ADMISSION_MS = 15 * MINUTE;
const DEVICE = "a".repeat(64);
const CREDENTIAL = "b".repeat(64);
const USER = 7;

interface AccountSession {
  createdAt: number;
  alive: boolean;
}

// The account service as the relay sees it: device credentials, one-time
// grants, and the lease check, all answered from this in-memory state.
class Accounts {
  deviceActive = true;
  readonly sessions = new Map<string, AccountSession>();
  private readonly grants = new Map<string, string>();
  private ticketSeq = 0;

  signIn(id: string): void {
    this.sessions.set(id, { createdAt: Date.now(), alive: true });
  }

  issueGrant(sessionId: string): string {
    const ticket = (++this.ticketSeq).toString(16).padStart(64, "0");
    this.grants.set(ticket, sessionId);
    return ticket;
  }

  async answer(path: string, body: Record<string, unknown>): Promise<Response> {
    if (path === "/remote/devices/authenticate") {
      if (!this.deviceActive || body.deviceCredential !== CREDENTIAL) return new Response(null, { status: 401 });
      return Response.json({ userId: USER, device: { id: DEVICE, publicKey: "A".repeat(43), capabilities: ["desktop"] } });
    }
    if (path === "/remote/grants/consume") {
      const sessionId = this.grants.get(String(body.ticket));
      this.grants.delete(String(body.ticket));
      const session = sessionId ? this.sessions.get(sessionId) : undefined;
      if (!sessionId || !session?.alive || !this.deviceActive) return new Response(null, { status: 401 });
      return Response.json({ grant: {
        userId: USER, targetDeviceId: DEVICE, scopes: ["desktop"], sessionId,
        reauthAt: new Date(session.createdAt + REAUTH_MS).toISOString(),
      } });
    }
    if (path === "/remote/leases/check") {
      const leases = body.leases as Array<{ sessionId?: string }>;
      return Response.json({ verdicts: leases.map((lease) => {
        if (!this.deviceActive) return "revoked";
        if (!lease.sessionId) return "active";
        const session = this.sessions.get(lease.sessionId);
        return session?.alive && Date.now() - session.createdAt < REAUTH_MS ? "active" : "reauth_required";
      }) });
    }
    return new Response(null, { status: 404 });
  }
}

class Relay {
  readonly state = new FakeState();
  readonly accounts = new Accounts();
  readonly room: RemoteSession;
  readonly env: Env;

  constructor() {
    this.env = {
      ACCOUNT_ORIGIN: "https://id.reasonix.io",
      ALLOWED_ORIGINS: "https://studio.reasonix.io",
      REMOTE_GATEWAY_TOKEN: "gateway-secret",
      ATTACHMENTS: {} as R2Bucket,
      REMOTE_SESSIONS: {
        idFromName: (name: string) => name as unknown as DurableObjectId,
        get: () => ({
          fetch: (input: RequestInfo, init?: RequestInit) =>
            this.room.fetch(input instanceof Request ? input : new Request(input, init)),
        }) as unknown as DurableObjectStub,
      } as unknown as DurableObjectNamespace,
    };
    this.room = new RemoteSession(this.state as unknown as DurableObjectState, this.env);
    vi.stubGlobal("fetch", async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      return this.accounts.answer(url.pathname, JSON.parse(String(init?.body ?? "{}")) as Record<string, unknown>);
    });
  }

  private async open(path: string, token: string): Promise<{ status: number; socket: FakeSocket | null }> {
    const before = this.state.sockets.length;
    const response = await worker.fetch!(new Request(`https://remote.reasonix.io${path}`, {
      headers: { authorization: `Bearer ${token}`, upgrade: "websocket" },
    }) as never, this.env, {} as ExecutionContext);
    const socket = this.state.sockets.length > before ? this.state.sockets.at(-1) ?? null : null;
    return { status: response.status, socket };
  }

  connectDevice() {
    return this.open(`/v1/devices/${DEVICE}/connect`, CREDENTIAL);
  }

  connectController(sessionId: string) {
    return this.open("/v1/sessions/connect", this.accounts.issueGrant(sessionId));
  }

  say(socket: FakeSocket, message: string): Promise<void> {
    return this.room.webSocketMessage(socket as unknown as WebSocket, message);
  }

  // Moves the clock, firing the object's alarm at each time it was set for.
  async advance(ms: number, each?: (now: number) => Promise<void> | void, stepMs = MINUTE): Promise<void> {
    const target = Date.now() + ms;
    while (Date.now() < target) {
      const step = Math.min(target, Date.now() + stepMs);
      while (this.state.alarmAt !== null && this.state.alarmAt <= step) {
        vi.setSystemTime(Math.max(this.state.alarmAt, Date.now()));
        this.state.alarmAt = null;
        await this.room.alarm();
      }
      vi.setSystemTime(step);
      await each?.(step);
    }
  }
}

function heartbeat(socket: FakeSocket): void {
  socket.autoResponseAt = new Date(Date.now());
}

function forwardedTo(device: FakeSocket, connectionId: string): number {
  return device.frames().filter((frame) => frame.type === "controller_message" && frame.connectionId === connectionId).length;
}

function connectionOf(device: FakeSocket): string {
  const frame = device.frames().filter((item) => item.type === "controller_connected").at(-1);
  return String(frame?.connectionId ?? "");
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(Date.parse("2026-09-28T00:00:00.000Z"));
  vi.stubGlobal("WebSocket", { OPEN: 1 });
  vi.stubGlobal("WebSocketPair", FakeWebSocketPair);
  vi.stubGlobal("WebSocketRequestResponsePair", FakeRequestResponsePair);
  vi.stubGlobal("Response", upgradeCapableResponse(Response));
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("relay connection lease, end to end", () => {
  it("keeps an active controller connected well past the old 15 minute admission and the idle period", async () => {
    const relay = new Relay();
    relay.accounts.signIn("1".repeat(64));
    const device = (await relay.connectDevice()).socket!;
    const phone = (await relay.connectController("1".repeat(64))).socket!;
    const id = connectionOf(device);

    await relay.advance(OLD_CONTROLLER_ADMISSION_MS + MINUTE, () => heartbeat(device));
    await relay.say(phone, "after-the-old-limit");
    expect(phone.closed).toBeNull();
    expect(forwardedTo(device, id)).toBe(1);

    await relay.advance(3 * HOUR, async () => {
      heartbeat(device);
      await relay.say(phone, "poll");
    });
    expect(phone.closed).toBeNull();
    expect(device.closed).toBeNull();
    expect(forwardedTo(device, id)).toBeGreaterThan(3 * 60);
  });

  it("closes a controller that stops talking once the idle period passes, and only it", async () => {
    const relay = new Relay();
    relay.accounts.signIn("1".repeat(64));
    const device = (await relay.connectDevice()).socket!;
    const phone = (await relay.connectController("1".repeat(64))).socket!;
    const id = connectionOf(device);

    await relay.advance(CONTROLLER_IDLE_MS - MINUTE, () => heartbeat(device));
    expect(phone.closed).toBeNull();

    await relay.advance(2 * MINUTE, () => heartbeat(device));
    expect(phone.closed?.code).toBe(CLOSE_IDLE);
    expect(device.closed).toBeNull();
    expect(device.frames()).toContainEqual(expect.objectContaining({ type: "controller_disconnected", connectionId: id }));
  });

  it("ends an active controller at the sign-in age limit and refuses a silent reconnect until the user signs in again", async () => {
    const relay = new Relay();
    relay.accounts.signIn("1".repeat(64));
    const device = (await relay.connectDevice()).socket!;
    const phone = (await relay.connectController("1".repeat(64))).socket!;

    await relay.advance(REAUTH_MS - MINUTE, async () => {
      heartbeat(device);
      await relay.say(phone, "poll");
    }, 10 * MINUTE);
    expect(phone.closed).toBeNull();

    await relay.advance(2 * MINUTE, async () => {
      heartbeat(device);
      if (phone.readyState === 1) await relay.say(phone, "poll");
    });
    expect(phone.closed?.code).toBe(CLOSE_REAUTH_REQUIRED);

    const retried = await relay.connectController("1".repeat(64));
    expect(retried.status).toBe(401);
    expect(retried.socket).toBeNull();

    relay.accounts.signIn("2".repeat(64));
    const fresh = await relay.connectController("2".repeat(64));
    expect(fresh.status).toBe(101);
    expect(fresh.socket?.readyState).toBe(1);
  });

  it("closes the device and its controllers the moment the device is removed", async () => {
    const relay = new Relay();
    relay.accounts.signIn("1".repeat(64));
    const device = (await relay.connectDevice()).socket!;
    const phone = (await relay.connectController("1".repeat(64))).socket!;
    const lateTicket = relay.accounts.issueGrant("1".repeat(64));

    relay.accounts.deviceActive = false;
    const revoke = await worker.fetch!(new Request("https://remote.reasonix.io/v1/devices/revoke", {
      method: "POST",
      headers: { "content-type": "application/json", "x-reasonix-gateway-token": "gateway-secret" },
      body: JSON.stringify({ deviceIds: [DEVICE] }),
    }) as never, relay.env, {} as ExecutionContext);

    expect(revoke.status).toBe(200);
    expect(phone.closed?.code).toBe(CLOSE_REVOKED);
    expect(device.closed?.code).toBe(CLOSE_REVOKED);
    expect((await relay.connectDevice()).status).toBe(401);
    const late = await worker.fetch!(new Request("https://remote.reasonix.io/v1/sessions/connect", {
      headers: { authorization: `Bearer ${lateTicket}`, upgrade: "websocket" },
    }) as never, relay.env, {} as ExecutionContext);
    expect(late.status).toBe(401);
  });

  it("refuses an admission that was authorized before the revocation reached the room", async () => {
    const relay = new Relay();
    relay.accounts.signIn("1".repeat(64));
    await relay.connectDevice();
    const admittedAt = Date.now();
    await relay.room.fetch(new Request("https://session.internal/revoke", { method: "POST", body: "{}" }));

    const response = await relay.room.fetch(new Request("https://session.internal/connect", {
      headers: {
        upgrade: "websocket",
        "x-reasonix-role": "controller",
        "x-reasonix-user-id": String(USER),
        "x-reasonix-device-id": DEVICE,
        "x-reasonix-admitted-at": String(admittedAt),
        "x-reasonix-reauth-at": String(Date.now() + HOUR),
      },
    }));
    expect(response.status).toBe(403);
  });

  it("closes only the signed-out session's controllers", async () => {
    const relay = new Relay();
    relay.accounts.signIn("1".repeat(64));
    relay.accounts.signIn("2".repeat(64));
    const device = (await relay.connectDevice()).socket!;
    const signedOut = (await relay.connectController("1".repeat(64))).socket!;
    const other = (await relay.connectController("2".repeat(64))).socket!;

    await worker.fetch!(new Request("https://remote.reasonix.io/v1/devices/revoke", {
      method: "POST",
      headers: { "content-type": "application/json", "x-reasonix-gateway-token": "gateway-secret" },
      body: JSON.stringify({ deviceIds: [DEVICE], sessionId: "1".repeat(64) }),
    }) as never, relay.env, {} as ExecutionContext);

    expect(signedOut.closed?.code).toBe(CLOSE_REAUTH_REQUIRED);
    expect(other.closed).toBeNull();
    expect(device.closed).toBeNull();
  });

  it("still disconnects within one check interval when the revocation push was lost", async () => {
    const relay = new Relay();
    relay.accounts.signIn("1".repeat(64));
    const device = (await relay.connectDevice()).socket!;
    const phone = (await relay.connectController("1".repeat(64))).socket!;

    relay.accounts.deviceActive = false;
    await relay.advance(REVALIDATE_MS + MINUTE, async () => {
      heartbeat(device);
      if (phone.readyState === 1) await relay.say(phone, "poll");
    });

    expect(phone.closed?.code).toBe(CLOSE_REVOKED);
    expect(device.closed?.code).toBe(CLOSE_REVOKED);
  });

  it("keeps controllers attached across a device reconnect and names them to the new device socket", async () => {
    const relay = new Relay();
    relay.accounts.signIn("1".repeat(64));
    const first = (await relay.connectDevice()).socket!;
    const phone = (await relay.connectController("1".repeat(64))).socket!;
    const id = connectionOf(first);

    const second = (await relay.connectDevice()).socket!;
    expect(first.closed?.code).toBe(4001);
    expect(phone.closed).toBeNull();
    expect(second.frames()).toContainEqual(expect.objectContaining({ type: "relay_hello", controllers: [id] }));

    await relay.say(phone, "still-here");
    expect(forwardedTo(second, id)).toBe(1);
  });

  it("keeps a heartbeating device online however long no controller talks", async () => {
    const relay = new Relay();
    const device = (await relay.connectDevice()).socket!;
    await relay.advance(6 * HOUR, () => heartbeat(device), 5 * MINUTE);
    expect(device.closed).toBeNull();
    const status = await relay.room.fetch(new Request("https://session.internal/status"));
    await expect(status.json()).resolves.toEqual({ online: true });
  });

  it("refuses a controller from a session signed out while its grant was in flight", async () => {
    const relay = new Relay();
    relay.accounts.signIn("1".repeat(64));
    await relay.connectDevice();
    const admittedAt = Date.now();
    await relay.room.fetch(new Request("https://session.internal/revoke", {
      method: "POST", body: JSON.stringify({ sessionId: "1".repeat(64) }),
    }));

    const response = await relay.room.fetch(new Request("https://session.internal/connect", {
      headers: {
        upgrade: "websocket",
        "x-reasonix-role": "controller",
        "x-reasonix-user-id": String(USER),
        "x-reasonix-device-id": DEVICE,
        "x-reasonix-session": "1".repeat(64),
        "x-reasonix-admitted-at": String(admittedAt),
        "x-reasonix-reauth-at": String(Date.now() + HOUR),
      },
    }));
    expect(response.status).toBe(401);
  });

  it("makes room for a new controller by closing one that went quiet, never an active one", async () => {
    const relay = new Relay();
    relay.accounts.signIn("1".repeat(64));
    const device = (await relay.connectDevice()).socket!;
    const phones: FakeSocket[] = [];
    for (let index = 0; index < 4; index += 1) phones.push((await relay.connectController("1".repeat(64))).socket!);

    expect((await relay.connectController("1".repeat(64))).status).toBe(429);

    await relay.advance(2 * MINUTE, async () => {
      heartbeat(device);
      for (const phone of phones.slice(1)) await relay.say(phone, "poll");
    });
    const fifth = await relay.connectController("1".repeat(64));
    expect(fifth.status).toBe(101);
    expect(phones[0]?.closed?.code).toBe(CLOSE_IDLE);
    expect(phones.slice(1).every((phone) => phone.closed === null)).toBe(true);
  });

  it("stops failing open once access has gone unchecked for an hour", async () => {
    const relay = new Relay();
    relay.accounts.signIn("1".repeat(64));
    const device = (await relay.connectDevice()).socket!;
    const phone = (await relay.connectController("1".repeat(64))).socket!;
    vi.stubGlobal("fetch", async () => new Response(null, { status: 503 }));

    await relay.advance(55 * MINUTE, async () => {
      heartbeat(device);
      await relay.say(phone, "poll");
    });
    expect(phone.closed).toBeNull();

    await relay.advance(10 * MINUTE, async () => {
      heartbeat(device);
      if (phone.readyState === 1) await relay.say(phone, "poll");
    });
    expect(phone.closed?.code).toBe(CLOSE_IDLE);
    expect(device.closed?.code).toBe(CLOSE_IDLE);
  });
});
