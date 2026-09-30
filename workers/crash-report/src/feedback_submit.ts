import type { Env } from "./env";
import { sendAlert } from "./alert";
import { decodeAttachment, deleteAttachments, storeAttachment } from "./feedback_attachments";
import { feedbackEnabled, tokenMatches } from "./feedback_auth";
import { readCappedText } from "./feedback_body";
import { installHash, installToken, ipHash, ipPrefix, newReceipt } from "./feedback_crypto";
import { jsonResponse, refuse } from "./feedback_http";
import { publicStatus } from "./feedback_read";
import { admit, firstBusyOfDay } from "./feedback_quota";
import { blockKey, capOverride, isBlocked, isTrusted } from "./feedback_blocks";
import { challengePassed } from "./feedback_turnstile";
import { FeedbackSubmit, type FeedbackSubmitInput } from "./feedback_schema";
import {
  GLOBAL_DAILY,
  MAX_BODY_BYTES,
  MAX_REQUEST_BYTES,
  PER_INSTALL_DAILY,
  PER_INSTALL_HOURLY,
  PER_IP_HOURLY,
  type FeedbackRow,
  type StoredAttachment,
} from "./feedback_types";
import { scrubSensitiveText } from "./scrub";

const RECEIPT_ATTEMPTS = 5;
const MAX_LINKS_BEFORE_HOLD = 3;
const RATE_LIMITED_MESSAGE = "submission limit reached";

function scrubEnv(env: FeedbackSubmitInput["env"]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(env)) if (typeof v === "string") out[k] = scrubSensitiveText(v);
  return out;
}

// A trusted install still goes through triage when its body is mostly links; a
// person describing a bug rarely pastes more than a few.
function tripsSpamGate(body: string): boolean {
  return (body.match(/https?:\/\//gi)?.length ?? 0) > MAX_LINKS_BEFORE_HOLD;
}

async function findByKey(env: Env, hash: string, key: string): Promise<FeedbackRow | null> {
  return env.DB.prepare("SELECT * FROM feedback WHERE install_hash = ? AND idempotency_key = ?").bind(hash, key).first<FeedbackRow>();
}

async function isKnownInstall(env: Env, hash: string): Promise<boolean> {
  return (await env.DB.prepare("SELECT 1 AS x FROM feedback WHERE install_hash = ? LIMIT 1").bind(hash).first()) !== null;
}

function receiptBody(row: FeedbackRow, token: string) {
  return {
    receipt: row.receipt,
    status: publicStatus(row),
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

async function storeAll(env: Env, decoded: ReturnType<typeof decodeAttachment>[]): Promise<StoredAttachment[]> {
  const stored: StoredAttachment[] = [];
  try {
    for (const [i, d] of decoded.entries()) if (d.ok && env.TELEMETRY_RAW) stored.push(await storeAttachment(env.TELEMETRY_RAW, d, i));
  } catch (err) {
    if (env.TELEMETRY_RAW) await deleteAttachments(env.TELEMETRY_RAW, stored);
    throw err;
  }
  return stored;
}

export async function handleSubmit(request: Request, env: Env): Promise<Response> {
  const secret = env.FEEDBACK_TOKEN_SECRET;
  if (!feedbackEnabled(env) || !secret) return refuse("feedback.disabled", "feedback is unavailable");
  const text = await readCappedText(request, MAX_REQUEST_BYTES);
  if (text === null) return refuse("feedback.too_large", "request too large");
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

  const hash = await installHash(secret, input.installId);
  const token = await installToken(secret, input.installId);
  const ip = request.headers.get("cf-connecting-ip") ?? "unknown";
  const ipKey = await ipHash(secret, ip);
  // Knowing the idempotency key proves the caller made the first attempt, so a
  // replay may recover the token even when the first response was lost.
  const replay = await findByKey(env, hash, input.idempotencyKey);
  if (replay) return jsonResponse(receiptBody(replay, token), 200);
  if ((await isKnownInstall(env, hash)) && !(await tokenMatches(secret, input.installId, request.headers.get("x-install-token") ?? ""))) {
    return refuse("feedback.bad_token", "install token missing or invalid");
  }
  // Only a new submission meets the block, after the replay and token answers a
  // blocked install gets exactly as an unblocked one would.
  if (await isBlocked(env, [`install:${hash}`, `ip:${ipKey}`], new Date())) return refuse("feedback.rate_limited", RATE_LIMITED_MESSAGE);
  if (!(await challengePassed(env, input.turnstileToken, ip))) return refuse("feedback.challenge_required", "verification required");

  if (env.FEEDBACK_LIMITER && !(await env.FEEDBACK_LIMITER.limit({ key: ipPrefix(ip) })).success) return refuse("feedback.rate_limited", RATE_LIMITED_MESSAGE);
  if (env.FEEDBACK_BUDGET_LIMITER && !(await env.FEEDBACK_BUDGET_LIMITER.limit({ key: "global" })).success) {
    return refuse("feedback.busy", "feedback is busy, try again later");
  }
  const decoded = input.attachments.map(decodeAttachment);
  for (const d of decoded) {
    if (!d.ok) {
      if (d.reason === "metadata") return refuse("feedback.image_metadata", "attachment carries metadata; re-encode the image");
      return refuse(d.reason === "too_large" ? "feedback.too_large" : "feedback.invalid", "attachment rejected");
    }
  }
  if (decoded.length > 0 && !env.TELEMETRY_RAW) return refuse("feedback.disabled", "attachments are unavailable");

  const trusted = await isTrusted(env, hash);
  const cap = (await capOverride(env)) ?? GLOBAL_DAILY;
  const admission = await admit(env, { ipKey, installHash: hash }, new Date(), {
    trusted,
    globalDaily: cap,
    ipHourly: PER_IP_HOURLY,
    installHourly: PER_INSTALL_HOURLY,
    installDaily: PER_INSTALL_DAILY,
  });
  if (admission === "limited") return refuse("feedback.rate_limited", RATE_LIMITED_MESSAGE);
  if (admission === "busy") {
    console.error("feedback: global daily cap reached");
    if (await firstBusyOfDay(env, new Date())) await sendAlert(env, `Feedback global daily cap (${cap}) reached or reserved for trusted installs; submissions are refused until 00:00 UTC.`);
    return refuse("feedback.busy", "feedback is busy, try again tomorrow");
  }


  const stored = await storeAll(env, decoded);
  const body = scrubSensitiveText(input.body);
  const at = new Date().toISOString();
  let outcome: Awaited<ReturnType<typeof insertWithReceipt>>;
  try {
    outcome = await insertWithReceipt(
      env,
      {
        install_hash: hash,
        category: input.category,
        body,
        display_name: scrubSensitiveText(input.displayName),
        contact: input.contact ?? "",
        env_json: JSON.stringify(scrubEnv(input.env)),
        attachments_json: JSON.stringify(stored),
        status: trusted && stored.length === 0 && !tripsSpamGate(body) ? "received" : "held",
        issue_number: null,
        issue_url: null,
        resolved_version: null,
        duplicate_of: null,
        created_at: at,
        updated_at: at,
      },
      input.idempotencyKey,
    );
  } catch (err) {
    if (env.TELEMETRY_RAW) await deleteAttachments(env.TELEMETRY_RAW, stored);
    throw err;
  }
  if (!outcome || "replay" in outcome) {
    if (env.TELEMETRY_RAW) await deleteAttachments(env.TELEMETRY_RAW, stored);
    if (!outcome) return refuse("feedback.disabled", "could not allocate a receipt");
    return jsonResponse(receiptBody(outcome.replay, token), 200);
  }
  return jsonResponse(receiptBody(outcome, token), 201);
}
