import { describe, expect, it } from "vitest";
import app from "../app";
import type { Bindings } from "../env";

function bindings(overrides: Partial<Bindings> = {}): Bindings {
  return {
    DB: {} as D1Database,
    APP_ORIGIN: "https://reasonix.io",
    ACCOUNT_ORIGIN: "https://id.reasonix.io",
    REMOTE_GATEWAY_ORIGIN: "https://remote.reasonix.io",
    ALLOWED_ORIGINS: "https://reasonix.io",
    COOKIE_DOMAIN: ".reasonix.io",
    EMAIL_PROVIDER: "stub",
    MAIL_FROM: "Reasonix <test@example.com>",
    ...overrides,
  };
}

describe("remote access HTTP boundaries", () => {
  it("does not expose registered devices without an account session", async () => {
    const response = await app.request("/me/devices", {}, bindings());
    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toMatchObject({ error: { code: "unauthorized" } });
  });

  it("fails closed when the gateway secret is not configured", async () => {
    const response = await app.request("/remote/grants/consume", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ ticket: "a".repeat(64) }),
    }, bindings());
    expect(response.status).toBe(503);
    await expect(response.json()).resolves.toMatchObject({ error: { code: "remote_unavailable" } });
  });

  it("rejects an incorrect gateway secret before reading a grant", async () => {
    const response = await app.request("/remote/grants/consume", {
      method: "POST",
      headers: {
        "content-type": "application/json",
        "x-reasonix-gateway-token": "wrong-secret",
      },
      body: JSON.stringify({ ticket: "a".repeat(64) }),
    }, bindings({ REMOTE_GATEWAY_TOKEN: "right-secret" }));
    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toMatchObject({ error: { code: "unauthorized_gateway" } });
  });

  it.each([
    ["an old browser session", { s_created: -(24 * 60 * 60 * 1000 + 1000), s_kind: "web" }],
    ["a fresh device-flow session", { s_created: 0, s_kind: "cli" }],
  ])("asks for a fresh sign-in before issuing a grant from %s", async (_name, session) => {
    const day = 24 * 60 * 60 * 1000;
    const db = {
      prepare() {
        return {
          bind() { return this; },
          async first() {
            return {
              id: 7, handle: "ada", email: "ada@example.com", email_verified: 1, password_hash: null,
              display_name: "", avatar_url: "", bio: "", role: "member", status: "active",
              created_at: "2026-01-01T00:00:00.000Z", updated_at: "2026-01-01T00:00:00.000Z",
              s_expires: new Date(Date.now() + day).toISOString(),
              s_created: new Date(Date.now() + session.s_created).toISOString(),
              s_kind: session.s_kind,
            };
          },
        };
      },
    } as unknown as D1Database;
    const response = await app.request("/me/remote-grants", {
      method: "POST",
      headers: { "content-type": "application/json", cookie: `rxid=${"a".repeat(64)}` },
      body: JSON.stringify({ targetDeviceId: "b".repeat(64), scopes: ["desktop"] }),
    }, bindings({ DB: db }));

    expect(response.status).toBe(403);
    await expect(response.json()).resolves.toMatchObject({ error: { code: "remote_reauth_required" } });
  });

  it("answers the relay's lease check only with the gateway secret", async () => {
    const leases = [{ userId: 7, deviceId: "a".repeat(64) }];
    const denied = await app.request("/remote/leases/check", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ leases }),
    }, bindings({ REMOTE_GATEWAY_TOKEN: "right-secret" }));
    expect(denied.status).toBe(401);

    const db = {
      prepare() {
        return { bind() { return this; }, async first() { return null; } };
      },
    } as unknown as D1Database;
    const response = await app.request("/remote/leases/check", {
      method: "POST",
      headers: { "content-type": "application/json", "x-reasonix-gateway-token": "right-secret" },
      body: JSON.stringify({ leases }),
    }, bindings({ REMOTE_GATEWAY_TOKEN: "right-secret", DB: db }));
    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ verdicts: ["revoked"] });
  });
});
