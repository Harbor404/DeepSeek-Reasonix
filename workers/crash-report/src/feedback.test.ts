// @ts-expect-error Node 22+ provides node:sqlite; Worker production code does not import it.
import { DatabaseSync } from "node:sqlite";
import { beforeEach, describe, expect, it } from "vitest";
import type { Env } from "./env";
import { handleFeedbackRoute } from "./feedback_routes";
import feedbackMigrationSQL from "../migrate-feedback.sql?raw";

const ADMIN = "admin-secret";

function d1(db: DatabaseSync): D1Database {
  const stmt = (sql: string, args: unknown[] = []) => ({
    bind: (...a: unknown[]) => stmt(sql, a),
    first: async () => db.prepare(sql).get(...args) ?? null,
    all: async () => ({ results: db.prepare(sql).all(...args) }),
    run: async () => ({ meta: { changes: Number(db.prepare(sql).run(...args).changes) } }),
  });
  return { prepare: (sql: string) => stmt(sql) } as unknown as D1Database;
}

function fakeR2() {
  const objects = new Map<string, { body: Uint8Array; contentType: string }>();
  const bucket = {
    put: async (k: string, body: Uint8Array, o: { httpMetadata: { contentType: string } }) => void objects.set(k, { body, contentType: o.httpMetadata.contentType }),
    get: async (k: string) => {
      const o = objects.get(k);
      return o ? { body: o.body, httpMetadata: { contentType: o.contentType } } : null;
    },
  };
  return { bucket: bucket as unknown as R2Bucket, objects };
}

let env: Env;
let objects: Map<string, unknown>;
let ipAllowed = true;

beforeEach(() => {
  const db = new DatabaseSync(":memory:");
  db.exec(feedbackMigrationSQL);
  const r2 = fakeR2();
  objects = r2.objects;
  ipAllowed = true;
  env = {
    DB: d1(db),
    FEEDBACK_R2: r2.bucket,
    FEEDBACK_LIMITER: { limit: async () => ({ success: ipAllowed }) },
    FEEDBACK_TOKEN_SECRET: "token-secret",
    FEEDBACK_ADMIN_TOKEN: ADMIN,
    FEEDBACK_ENABLED: "true",
  } as unknown as Env;
});

const call = (path: string, init: RequestInit = {}) => handleFeedbackRoute(new Request(`https://crash.test${path}`, init), env) as Promise<Response>;
const post = (path: string, body: unknown, headers: Record<string, string> = {}) =>
  call(path, { method: "POST", body: JSON.stringify(body), headers: { "content-type": "application/json", ...headers } });
const admin = { authorization: `Bearer ${ADMIN}` };

const PNG_B64 = btoa(String.fromCharCode(0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3));
let seq = 0;
const submission = (over: Record<string, unknown> = {}) => ({
  idempotencyKey: `key-${++seq}-aaaaaaaa`,
  installId: "install-aaaaaaaaaaaaaaaa",
  category: "bug",
  body: "the composer freezes",
  displayName: "Ada",
  env: { version: "v2.24.0", surface: "studio" },
  ...over,
});
const errCode = async (r: Response) => ((await r.json()) as { error: { code: string } }).error.code;

describe("POST /v1/feedback", () => {
  it("accepts a submission and returns a receipt and install token", async () => {
    const res = await post("/v1/feedback", submission());
    expect(res.status).toBe(201);
    const j = (await res.json()) as { receipt: string; status: string; installToken: string };
    expect(j.receipt).toMatch(/^FB-[0-9A-HJKMNP-TV-Z]{4}-[0-9A-HJKMNP-TV-Z]{4}$/);
    expect(j.status).toBe("received");
    expect(j.installToken.length).toBeGreaterThan(20);
  });

  it("replays an idempotency key without creating a second row", async () => {
    const s = submission();
    const a = (await (await post("/v1/feedback", s)).json()) as { receipt: string };
    const res = await post("/v1/feedback", s);
    expect(res.status).toBe(200);
    expect(((await res.json()) as { receipt: string }).receipt).toBe(a.receipt);
  });

  it("caps an install at three submissions per hour", async () => {
    for (let i = 0; i < 3; i++) expect((await post("/v1/feedback", submission())).status).toBe(201);
    const res = await post("/v1/feedback", submission());
    expect(res.status).toBe(429);
    expect(await errCode(res)).toBe("feedback.rate_limited");
  });

  it("honours the per-IP limiter", async () => {
    ipAllowed = false;
    expect(await errCode(await post("/v1/feedback", submission()))).toBe("feedback.rate_limited");
  });

  it("rejects an oversize body and an oversize attachment", async () => {
    const big = await post("/v1/feedback", submission({ body: "x".repeat(8193) }));
    expect(big.status).toBe(413);
    expect(await errCode(big)).toBe("feedback.too_large");
    const png = btoa(String.fromCharCode(0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a) + "a".repeat(2 * 1024 * 1024));
    const res = await post("/v1/feedback", submission({ attachments: [{ name: "a.png", contentType: "image/png", dataBase64: png }] }));
    expect(await errCode(res)).toBe("feedback.too_large");
  });

  it("rejects an attachment whose bytes do not match its type", async () => {
    const res = await post("/v1/feedback", submission({ attachments: [{ name: "a.png", contentType: "image/png", dataBase64: btoa("not an image") }] }));
    expect(res.status).toBe(400);
    expect(await errCode(res)).toBe("feedback.invalid");
    expect(objects.size).toBe(0);
  });

  it("stores a valid image and serves it with nosniff", async () => {
    const res = await post("/v1/feedback", submission({ attachments: [{ name: "shot.png", contentType: "image/png", dataBase64: PNG_B64 }] }));
    expect(res.status).toBe(201);
    const [key] = [...objects.keys()];
    expect(key.startsWith("feedback/")).toBe(true);
    const served = await call(`/v1/feedback/attachments/${key.slice("feedback/".length)}`);
    expect(served.status).toBe(200);
    expect(served.headers.get("content-type")).toBe("image/png");
    expect(served.headers.get("x-content-type-options")).toBe("nosniff");
    expect(served.headers.get("content-disposition")).toBe("inline");
    expect((await call("/v1/feedback/attachments/zzzzzzzzzzzzzzzzzzzz")).status).toBe(404);
  });

  it("scrubs secrets from the body server-side", async () => {
    await post("/v1/feedback", submission({ body: "my key is sk-abcdefghijklmnopqrstuvwx ok" }));
    const list = (await (await call("/v1/admin/feedback/pending", { headers: admin })).json()) as { items: { body: string }[] };
    expect(list.items[0].body).not.toContain("sk-abcdefgh");
  });

  it("stays disabled while either secret is absent", async () => {
    env.FEEDBACK_ADMIN_TOKEN = undefined;
    expect(await errCode(await post("/v1/feedback", submission()))).toBe("feedback.disabled");
    env.FEEDBACK_ADMIN_TOKEN = ADMIN;
    env.FEEDBACK_TOKEN_SECRET = undefined;
    expect(await errCode(await call("/v1/feedback/mine"))).toBe("feedback.disabled");
  });

  it("answers feedback.disabled when the kill switch is off", async () => {
    env.FEEDBACK_ENABLED = "false";
    const res = await post("/v1/feedback", submission());
    expect(res.status).toBe(503);
    expect(await errCode(res)).toBe("feedback.disabled");
  });
});

describe("GET /v1/feedback/mine", () => {
  it("rejects a wrong token", async () => {
    const res = await call("/v1/feedback/mine", { headers: { "x-install-id": "install-aaaaaaaaaaaaaaaa", "x-install-token": "nope" } });
    expect(res.status).toBe(401);
    expect(await errCode(res)).toBe("feedback.bad_token");
  });

  it("lists only the caller's own items", async () => {
    const a = (await (await post("/v1/feedback", submission())).json()) as { installToken: string };
    const b = (await (await post("/v1/feedback", submission({ installId: "install-bbbbbbbbbbbbbbbb", body: "other" }))).json()) as { installToken: string };
    const mine = async (id: string, token: string) =>
      ((await (await call("/v1/feedback/mine", { headers: { "x-install-id": id, "x-install-token": token } })).json()) as { items: { titleSnippet: string }[] }).items;
    expect((await mine("install-aaaaaaaaaaaaaaaa", a.installToken)).map((i) => i.titleSnippet)).toEqual(["the composer freezes"]);
    expect((await mine("install-bbbbbbbbbbbbbbbb", b.installToken)).map((i) => i.titleSnippet)).toEqual(["other"]);
  });
});

describe("admin flow", () => {
  const receiptOf = async (s = submission()) => ((await (await post("/v1/feedback", s)).json()) as { receipt: string }).receipt;
  const status = (r: string, body: unknown) => post(`/v1/admin/feedback/${r}/status`, body, admin);

  it("requires the admin bearer token", async () => {
    expect((await call("/v1/admin/feedback/pending")).status).toBe(401);
    expect((await call("/v1/admin/feedback/pending", { headers: { authorization: "Bearer wrong" } })).status).toBe(401);
  });

  it("walks received -> recorded -> in_progress -> fixed and never contacts leak", async () => {
    const r = await receiptOf(submission({ contact: "me@example.test" }));
    const pend = await call("/v1/admin/feedback/pending?limit=5", { headers: admin });
    const text = await pend.text();
    expect(text).toContain(r);
    expect(text).not.toContain("example.test");
    expect((await post(`/v1/admin/feedback/${r}/recorded`, { issueNumber: 7, issueUrl: "https://github.com/o/r/issues/7" }, admin)).status).toBe(200);
    expect((await post(`/v1/admin/feedback/${r}/recorded`, { issueNumber: 7, issueUrl: "https://github.com/o/r/issues/7" }, admin)).status).toBe(200);
    expect(((await (await call("/v1/admin/feedback/pending", { headers: admin })).json()) as { items: unknown[] }).items).toHaveLength(0);
    expect((await status(r, { status: "in_progress" })).status).toBe(200);
    const open = (await (await call("/v1/admin/feedback/open", { headers: admin })).json()) as { items: { receipt: string; issueNumber: number }[] };
    expect(open.items).toEqual([expect.objectContaining({ receipt: r, issueNumber: 7 })]);
    expect((await status(r, { status: "fixed" })).status).toBe(400);
    expect((await status(r, { status: "fixed", resolvedVersion: "next" })).status).toBe(200);
  });

  it("upgrades fixed(next) to a concrete version once and lists it while awaiting a tag", async () => {
    const r = await receiptOf();
    await post(`/v1/admin/feedback/${r}/recorded`, { issueNumber: 3, issueUrl: "https://github.com/o/r/issues/3" }, admin);
    await status(r, { status: "fixed", resolvedVersion: "next" });
    const open = async () => ((await (await call("/v1/admin/feedback/open", { headers: admin })).json()) as { items: { receipt: string }[] }).items;
    expect(await open()).toEqual([expect.objectContaining({ receipt: r })]);
    expect(await errCode(await status(r, { status: "fixed", resolvedVersion: "latest" }))).toBe("feedback.bad_transition");
    expect((await status(r, { status: "fixed", resolvedVersion: "v2.25.0" })).status).toBe(200);
    expect(await open()).toHaveLength(0);
    expect((await status(r, { status: "fixed", resolvedVersion: "v2.25.0" })).status).toBe(200);
    expect(await errCode(await status(r, { status: "fixed", resolvedVersion: "v2.26.0" }))).toBe("feedback.bad_transition");
    expect(await errCode(await status(r, { status: "fixed", resolvedVersion: "next" }))).toBe("feedback.bad_transition");
  });

  it("refuses backward and terminal-to-terminal transitions", async () => {
    const r = await receiptOf();
    expect(await errCode(await status(r, { status: "in_progress" }))).toBe("feedback.bad_transition");
    await post(`/v1/admin/feedback/${r}/recorded`, { issueNumber: 1, issueUrl: "https://github.com/o/r/issues/1" }, admin);
    await status(r, { status: "wontfix" });
    expect(await errCode(await status(r, { status: "in_progress" }))).toBe("feedback.bad_transition");
    expect(await errCode(await status(r, { status: "fixed", resolvedVersion: "v1.0.0" }))).toBe("feedback.bad_transition");
  });

  it("hides held feedback from the converter until released", async () => {
    const links = Array.from({ length: 5 }, (_, i) => `https://spam${i}.test`).join(" ");
    const r = await receiptOf(submission({ body: links }));
    const items = async () => ((await (await call("/v1/admin/feedback/pending", { headers: admin })).json()) as { items: unknown[] }).items;
    expect(await items()).toHaveLength(0);
    expect(await errCode(await post(`/v1/admin/feedback/${r}/recorded`, { issueNumber: 2, issueUrl: "https://github.com/o/r/issues/2" }, admin))).toBe("feedback.bad_transition");
    expect((await post(`/v1/admin/feedback/${r}/release`, {}, admin)).status).toBe(200);
    expect(await items()).toHaveLength(1);
  });
});
