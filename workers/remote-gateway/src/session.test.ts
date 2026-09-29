import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RemoteSession } from "./session";
import type { Env } from "./env";
import { FakeRequestResponsePair, FakeSocket, FakeState } from "./fake_room";

function room(state: FakeState): RemoteSession {
  return new RemoteSession(state as unknown as DurableObjectState, {} as Env);
}

beforeEach(() => {
  vi.stubGlobal("WebSocket", { OPEN: 1 });
  vi.stubGlobal("WebSocketRequestResponsePair", FakeRequestResponsePair);
});

afterEach(() => vi.unstubAllGlobals());

describe("remote session presence", () => {
  it("reports a live device socket without extending its lease", async () => {
    const state = new FakeState();
    const socket = new FakeSocket();
    const lease = {
      role: "device", userId: 7, deviceId: "a".repeat(64), scopes: ["desktop"], connectionId: null,
      sessionId: null, admittedAt: 1, lastActiveAt: 1, reauthAt: null,
    };
    socket.serializeAttachment(lease);
    state.acceptWebSocket(socket, ["device"]);

    const response = await room(state).fetch(new Request("https://session.internal/status"));

    await expect(response.json()).resolves.toEqual({ online: true });
    expect(socket.deserializeAttachment()).toEqual(lease);
  });

  it("does not report a closed historical socket as online", async () => {
    const state = new FakeState();
    const socket = new FakeSocket();
    socket.readyState = 3;
    state.acceptWebSocket(socket, ["device"]);

    const response = await room(state).fetch(new Request("https://session.internal/status"));

    await expect(response.json()).resolves.toEqual({ online: false });
  });

  it("answers a device's JSON heartbeat even when the runtime did not", async () => {
    const state = new FakeState();
    const socket = new FakeSocket();
    socket.serializeAttachment({
      role: "device", userId: 7, deviceId: "a".repeat(64), scopes: [], connectionId: null,
      sessionId: null, admittedAt: 1, lastActiveAt: 1, reauthAt: null,
    });
    state.acceptWebSocket(socket, ["device"]);

    await room(state).webSocketMessage(socket as unknown as WebSocket, JSON.stringify({ type: "heartbeat" }));

    expect(socket.closed).toBeNull();
    expect(socket.sent).toEqual(['{"type":"heartbeat_ack"}']);
    expect((socket.deserializeAttachment() as { lastActiveAt: number }).lastActiveAt).toBeGreaterThan(1);
  });
});
