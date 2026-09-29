import { afterEach, describe, expect, it, vi } from "vitest";
import type { Bindings } from "./env";
import { disconnectRemote, remoteDevicePresence } from "./remoteGateway";

const env = {
  REMOTE_GATEWAY_ORIGIN: "https://remote.reasonix.io/",
  REMOTE_GATEWAY_TOKEN: "gateway-secret",
} as Bindings;

afterEach(() => vi.unstubAllGlobals());

describe("remote device presence", () => {
  it("returns only devices confirmed online by the gateway", async () => {
    const first = "1".repeat(64);
    const second = "2".repeat(64);
    const fetchMock = vi.fn(async () => Response.json({ devices: [
      { id: first, online: true },
      { id: second, online: false },
    ] }));
    vi.stubGlobal("fetch", fetchMock);

    const result = await remoteDevicePresence(env, [first, second]);

    expect(result.available).toBe(true);
    expect([...result.onlineIds]).toEqual([first]);
    expect(fetchMock).toHaveBeenCalledWith("https://remote.reasonix.io/v1/devices/status", expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ deviceIds: [first, second] }),
    }));
  });

  it("fails closed when gateway presence cannot be checked", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(null, { status: 503 })));
    const result = await remoteDevicePresence(env, ["1".repeat(64)]);
    expect(result).toEqual({ onlineIds: new Set(), available: false });
  });
});

describe("remote revocation push", () => {
  it("asks the gateway to close a removed device's connections", async () => {
    const fetchMock = vi.fn(async () => Response.json({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(disconnectRemote(env, ["1".repeat(64)])).resolves.toBe(true);
    expect(fetchMock).toHaveBeenCalledWith("https://remote.reasonix.io/v1/devices/revoke", expect.objectContaining({
      method: "POST",
      headers: expect.objectContaining({ "x-reasonix-gateway-token": "gateway-secret" }),
      body: JSON.stringify({ deviceIds: ["1".repeat(64)] }),
    }));
  });

  it("names the signed-out session so only its controllers close", async () => {
    const fetchMock = vi.fn(async () => Response.json({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);

    await disconnectRemote(env, ["1".repeat(64)], "2".repeat(64));
    expect(fetchMock).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({
      body: JSON.stringify({ deviceIds: ["1".repeat(64)], sessionId: "2".repeat(64) }),
    }));
  });

  it("retries once and reports an undelivered disconnect without throwing", async () => {
    const fetchMock = vi.fn(async () => { throw new Error("offline"); });
    vi.stubGlobal("fetch", fetchMock);
    await expect(disconnectRemote(env, ["1".repeat(64)])).resolves.toBe(false);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
