import type { Env } from "./env";
import { ATTACHMENT_PREFIX } from "./feedback_attachments";
import { UNCONVERTED_RETENTION_DAYS, type StoredAttachment } from "./feedback_types";

// One cron invocation shares a subrequest budget with the rest of the purge, so
// the sweep is bounded (at most ~3 subrequests per chunk) and logs when it stops early.
const AUDIT_RETENTION_DAYS = 180;
const CHUNK = 25;
const MAX_CHUNKS = 4;
const STATUSES = ["held", "received", "needs_info", "answered", "rejected"] as const;

interface StaleRow {
  receipt: string;
  attachments_json: string;
}

async function purgeChunk(env: Env, status: string, cutoff: string): Promise<number> {
  const { results } = await env.DB.prepare("SELECT receipt, attachments_json FROM feedback WHERE status = ? AND created_at < ? AND updated_at < ? ORDER BY created_at ASC LIMIT ?")
    .bind(status, cutoff, cutoff, CHUNK)
    .all<StaleRow>();
  if (results.length === 0) return 0;
  const attachments = results.flatMap((r) => JSON.parse(r.attachments_json) as StoredAttachment[]);
  if (env.TELEMETRY_RAW) await env.TELEMETRY_RAW.delete(attachments.map((a) => ATTACHMENT_PREFIX + a.key));
  const marks = results.map(() => "?").join(",");
  const receipts = results.map((r) => r.receipt);
  await env.DB.prepare(`DELETE FROM feedback_replies WHERE receipt IN (${marks})`).bind(...receipts).run();
  await env.DB.prepare(`DELETE FROM feedback_public_images WHERE receipt IN (${marks})`).bind(...receipts).run();
  await env.DB.prepare(`DELETE FROM feedback WHERE status = ? AND receipt IN (${marks})`)
    .bind(status, ...receipts)
    .run();
  return results.length;
}

// Age counts from the last update, so a reply keeps a report alive. Feedback that never became an issue (held, unreleased, answered, rejected) is dropped with its images and replies;
// recorded and later rows stay because their issues link to the images.
export async function purgeStaleFeedback(env: Env): Promise<void> {
  const now = Date.now();
  const cutoff = new Date(now - UNCONVERTED_RETENTION_DAYS * 86_400_000).toISOString();
  const quotaDay = new Date(now - 2 * 86_400_000).toISOString().slice(0, 10);
  try {
    for (const status of STATUSES) {
      let purged = 0;
      let last = 0;
      for (let i = 0; i < MAX_CHUNKS; i++) {
        last = await purgeChunk(env, status, cutoff);
        purged += last;
        if (last < CHUNK) break;
      }
      const truncated = last === CHUNK;
      console.log(`retention: purged ${purged} ${status} feedback rows${truncated ? " (truncated, more remain)" : ""}`);
    }
    await env.DB.prepare("DELETE FROM feedback_quota WHERE day < ?").bind(quotaDay).run();
    await env.DB.prepare("DELETE FROM feedback_trust WHERE expires_at < ?").bind(new Date(now).toISOString()).run();
    await env.DB.prepare("DELETE FROM feedback_blocks WHERE expires_at IS NOT NULL AND expires_at < ?").bind(new Date(now).toISOString()).run();
    const auditCutoff = new Date(now - AUDIT_RETENTION_DAYS * 86_400_000).toISOString();
    await env.DB.prepare("DELETE FROM feedback_audit WHERE at < ?").bind(auditCutoff).run();
  } catch (err) {
    console.error("retention: feedback purge failed", err);
  }
}
