import type { Env } from "./env";
import { attachmentUrl } from "./feedback_attachments";
import { verifyInstall } from "./feedback_auth";
import { jsonResponse } from "./feedback_http";
import type { FeedbackRow, StoredAttachment } from "./feedback_types";

const MINE_LIMIT = 50;
const SNIPPET_CHARS = 80;

function publicStatus(row: FeedbackRow) {
  return row.status === "held" ? "received" : row.status;
}

export async function handleMine(request: Request, env: Env): Promise<Response> {
  const who = await verifyInstall(request, env);
  if (who instanceof Response) return who;
  const { results } = await env.DB.prepare("SELECT * FROM feedback WHERE install_hash = ? ORDER BY created_at DESC LIMIT ?")
    .bind(who.installHash, MINE_LIMIT)
    .all<FeedbackRow>();
  return jsonResponse({
    items: results.map((r) => ({
      receipt: r.receipt,
      category: r.category,
      titleSnippet: [...r.body].slice(0, SNIPPET_CHARS).join(""),
      status: publicStatus(r),
      issueNumber: r.issue_number,
      issueUrl: r.issue_url,
      resolvedVersion: r.resolved_version,
      duplicateOf: r.duplicate_of,
      createdAt: r.created_at,
      updatedAt: r.updated_at,
    })),
  });
}

export function pendingItem(row: FeedbackRow, origin: string) {
  const attachments = JSON.parse(row.attachments_json) as StoredAttachment[];
  return {
    receipt: row.receipt,
    category: row.category,
    body: row.body,
    displayName: row.display_name,
    env: JSON.parse(row.env_json) as Record<string, string>,
    attachments: attachments.map((a) => ({ name: a.name, contentType: a.contentType, url: attachmentUrl(origin, a.key) })),
    createdAt: row.created_at,
  };
}
