import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { coarseHash, resetTrace, trace } from "./trace";

let lines: Array<Record<string, unknown>>;

beforeEach(() => {
  resetTrace();
  lines = [];
  vi.spyOn(console, "log").mockImplementation((line: string) => { lines.push(JSON.parse(line)); });
});

afterEach(() => vi.restoreAllMocks());

describe("gateway trace", () => {
  it("hashes to eight hex characters that cannot be turned back into the id", () => {
    const id = "a1".repeat(32);
    const hashed = coarseHash(id);
    expect(hashed).toMatch(/^[0-9a-f]{8}$/);
    expect(hashed).toBe(coarseHash(id));
    expect(hashed).not.toBe(coarseHash("a2".repeat(32)));
  });

  it("emits one structured line per event", () => {
    trace("handshake_rejected", { code: "invalid_grant", status: 401 });
    expect(lines).toEqual([{ svc: "remote-gateway", event: "handshake_rejected", code: "invalid_grant", status: 401 }]);
  });

  it("caps a hot code per window and reports what it skipped in the next", () => {
    for (let index = 0; index < 100; index += 1) trace("handshake_rejected", { code: "invalid_token" }, 1_000);
    trace("handshake_rejected", { code: "rate_limited" }, 1_000);
    expect(lines.filter((line) => line.code === "invalid_token")).toHaveLength(20);
    expect(lines.filter((line) => line.code === "rate_limited")).toHaveLength(1);

    trace("handshake_rejected", { code: "invalid_token" }, 61_001);
    expect(lines.at(-1)).toMatchObject({ code: "invalid_token", suppressed: 80 });
  });

  it("never throws into the caller when logging fails", () => {
    vi.spyOn(console, "log").mockImplementation(() => { throw new Error("sink down"); });
    expect(() => trace("handshake_rejected", { code: "invalid_token" })).not.toThrow();
  });
});
