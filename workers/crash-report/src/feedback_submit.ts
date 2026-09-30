import type { Env } from "./env";
import { decodeAttachment, storeAttachment } from "./feedback_attachments";
import { installToken, newReceipt, sha256Hex } from "./feedback_crypto";
import { jsonResponse, refuse } from "./feedback_http";
import { FeedbackSubmit, type FeedbackSubmitInput } from "./feedback_schema";
import {
  MAX_ATTACHMENTS,
  MAX_BODY_BYTES,
  MAX_REQUEST_BYTES,
  PER_INSTALL_DAILY,
  PER_INSTALL_HOURLY,
  type FeedbackRow,
  type StoredAttachment,
} from "./feedback_types";
import { scrubSensitiveText } from "./scrub";

const RECEIPT_ATTEMPTS = 5;
const MAX_LINKS_BEFORE_HOLD = 3;

function scrubEnv(env: FeedbackSubmitInput["env"]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(env)) if (typeof v === "string") out[k] = scrubSensitiveText(v);
  return out;
}

// A body that is mostly links is the one structural spam signal; a person
// describing a bug rarely pastes more than a few.
function tripsSpamGate(body: string): boolean {
  return (body.match(/https?:\/\//gi)?.length ?? 0) > MAX_LINKS_BEFORE_HOLD;
}

async function findByKey(env: Env, installHash: string, key: string): Promise<FeedbackRow | null> {
  return env.DB.prepare("SELECT * FROM feedback WHERE install_hash = ? AND idempotency_key = ?")
    .bind(installHash, key)
    .first<FeedbackRow>();
}

async function overInstallLimit(env: Env, installHash: string, now: number): Promise<boolean> {
  const dayAgo = new Date(now - 86_400_000).toISOString();
  const hourAgo = new Date(now - 3_600_000).toISOString();
  const { results } = await env.DB.prepare(
    "SELECT created_at FROM feedback WHERE install_hash = ? AND created_at >= ? ORDER BY created_at DESC LIMIT ?",
  )
    .bind(installHash, dayAgo, PER_INSTALL_DAILY)
    .all<{ created_at: string }>();
  return results.length >= PER_INSTALL_DAILY || results.filter((r) => r.created_at >= hourAgo).length >= PER_INSTALL_HOURLY;
}

function receiptBody(row: FeedbackRow, token: string) {
  return {
    receipt: row.receipt,
    status: row.status === "held" ? "received" : row.status,
    installToken: token,
    createdAt: row.created_at,
  };
}

async function insertWithReceipt(env: Env, row: Omit<FeedbackRow, "receipt">, key: string): Promise<FeedbackRow | { replay: FeedbackRow } | null> {
  for (let i = 0; i < RECEIPT_ATTEMPTS; i++) {
    const receipt = newReceipt();
    try {
      await env.DB.prepare(
        `INSERT INTO feedback (receipt, install_hash, idempotency_key, category, body, display_name, contact, env_json,
           attachments_json, status, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
      )
        .bind(receipt, row.install_hash, key, row.category, row.body, row.display_name, row.contact, row.env_json, row.attachments_json, row.status, row.created_at, row.updated_at)
        .run();
      return { ...row, receipt };
    } catch (err) {
      const existing = await findByKey(env, row.install_hash, key);
      if (existing) return { replay: existing };
      if (i === RECEIPT_ATTEMPTS - 1) throw err;
    }
  }
  return null;
}

export async function handleSubmit(request: Request, env: Env): Promise<Response> {
  if (env.FEEDBACK_ENABLED === "false" || !env.FEEDBACK_TOKEN_SECRET) return refuse("feedback.disabled", "feedback is unavailable");
  const ip = request.headers.get("cf-connecting-ip") ?? "unknown";
  if (env.FEEDBACK_LIMITER && !(await env.FEEDBACK_LIMITER.limit({ key: ip })).success) return refuse("feedback.rate_limited", "too many submissions");
  if (env.FEEDBACK_BUDGET_LIMITER && !(await env.FEEDBACK_BUDGET_LIMITER.limit({ key: "global" })).success) {
    return refuse("feedback.rate_limited", "feedback is busy, try again later");
  }
  const declared = Number(request.headers.get("content-length") ?? 0);
  if (declared > MAX_REQUEST_BYTES) return refuse("feedback.too_large", "request too large");
  const text = await request.text();
  if (text.length > MAX_REQUEST_BYTES) return refuse("feedback.too_large", "request too large");
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    return refuse("feedback.invalid", "body is not valid JSON");
  }
  const parsed = FeedbackSubmit.safeParse(raw);
  if (!parsed.success) return refuse("feedback.invalid", "request does not match the feedback schema");
  const input = parsed.data;
  if (new TextEncoder().encode(input.body).length > MAX_BODY_BYTES) return refuse("feedback.too_large", "body exceeds 8192 bytes");
  if (input.attachments.length > MAX_ATTACHMENTS) return refuse("feedback.invalid", "too many attachments");

  const installHash = await sha256Hex(input.installId);
  const token = await installToken(env.FEEDBACK_TOKEN_SECRET, input.installId);
  const replay = await findByKey(env, installHash, input.idempotencyKey);
  if (replay) return jsonResponse(receiptBody(replay, token), 200);

  const decoded = input.attachments.map(decodeAttachment);
  for (const d of decoded) {
    if (!d.ok) return refuse(d.reason === "too_large" ? "feedback.too_large" : "feedback.invalid", "attachment rejected");
  }
  if (decoded.length > 0 && !env.FEEDBACK_R2) return refuse("feedback.disabled", "attachments are unavailable");

  const now = Date.now();
  if (await overInstallLimit(env, installHash, now)) return refuse("feedback.rate_limited", "submission limit reached for this install");

  const stored: StoredAttachment[] = [];
  for (const d of decoded) if (d.ok && env.FEEDBACK_R2) stored.push(await storeAttachment(env.FEEDBACK_R2, d));

  const body = scrubSensitiveText(input.body);
  const at = new Date(now).toISOString();
  const outcome = await insertWithReceipt(
    env,
    {
      install_hash: installHash,
      category: input.category,
      body,
      display_name: scrubSensitiveText(input.displayName),
      contact: input.contact ?? "",
      env_json: JSON.stringify(scrubEnv(input.env)),
      attachments_json: JSON.stringify(stored),
      status: tripsSpamGate(body) ? "held" : "received",
      issue_number: null,
      issue_url: null,
      resolved_version: null,
      duplicate_of: null,
      created_at: at,
      updated_at: at,
    },
    input.idempotencyKey,
  );
  if (!outcome) return refuse("feedback.disabled", "could not allocate a receipt");
  if ("replay" in outcome) return jsonResponse(receiptBody(outcome.replay, token), 200);
  return jsonResponse(receiptBody(outcome, token), 201);
}
