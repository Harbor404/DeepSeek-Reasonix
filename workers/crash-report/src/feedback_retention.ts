import type { Env } from "./env";
import { ATTACHMENT_PREFIX } from "./feedback_attachments";
import { UNCONVERTED_RETENTION_DAYS, type StoredAttachment } from "./feedback_types";

// One cron invocation shares a subrequest budget with the rest of the purge, so
// the sweep is bounded (at most ~3 subrequests per chunk) and logs when it stops early.
const CHUNK = 25;
const MAX_CHUNKS = 4;
const STATUSES = ["held", "received"] as const;

interface StaleRow {
  receipt: string;
  attachments_json: string;
}

async function purgeChunk(env: Env, status: string, cutoff: string): Promise<number> {
  const { results } = await env.DB.prepare("SELECT receipt, attachments_json FROM feedback WHERE status = ? AND created_at < ? ORDER BY created_at ASC LIMIT ?")
    .bind(status, cutoff, CHUNK)
    .all<StaleRow>();
  if (results.length === 0) return 0;
  const attachments = results.flatMap((r) => JSON.parse(r.attachments_json) as StoredAttachment[]);
  if (env.TELEMETRY_RAW) await env.TELEMETRY_RAW.delete(attachments.map((a) => ATTACHMENT_PREFIX + a.key));
  const marks = results.map(() => "?").join(",");
  await env.DB.prepare(`DELETE FROM feedback WHERE status = ? AND receipt IN (${marks})`)
    .bind(status, ...results.map((r) => r.receipt))
    .run();
  return results.length;
}

// Feedback the converter never turned into an issue is dropped with its images;
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
  } catch (err) {
    console.error("retention: feedback purge failed", err);
  }
}
