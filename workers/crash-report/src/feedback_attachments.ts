import type { Env } from "./env";
import { inspectImage, sniffImage, type ImageKind } from "./feedback_image";
import { newAttachmentKey } from "./feedback_crypto";
import { MAX_ATTACHMENT_BYTES, type StoredAttachment } from "./feedback_types";

export const ATTACHMENT_PREFIX = "feedback/";
export const ATTACHMENT_ROUTE = "/v1/feedback/attachments/";

const TYPE_OF: Record<ImageKind, string> = { png: "image/png", jpeg: "image/jpeg" };
const EXT_OF: Record<ImageKind, string> = { png: "png", jpeg: "jpg" };

export interface AttachmentInput {
  name: string;
  contentType: string;
  dataBase64: string;
}

export type DecodedAttachment =
  | { ok: true; contentType: string; ext: string; bytes: Uint8Array }
  | { ok: false; reason: "too_large" | "invalid" | "metadata" };

export function decodeAttachment(a: AttachmentInput): DecodedAttachment {
  if (a.dataBase64.length > Math.ceil((MAX_ATTACHMENT_BYTES * 4) / 3) + 4) return { ok: false, reason: "too_large" };
  let bytes: Uint8Array;
  try {
    bytes = Uint8Array.from(atob(a.dataBase64), (c) => c.charCodeAt(0));
  } catch {
    return { ok: false, reason: "invalid" };
  }
  if (bytes.length > MAX_ATTACHMENT_BYTES) return { ok: false, reason: "too_large" };
  const kind = sniffImage(bytes);
  if (!kind || TYPE_OF[kind] !== a.contentType) return { ok: false, reason: "invalid" };
  const verdict = inspectImage(bytes, kind);
  if (verdict === "metadata") return { ok: false, reason: "metadata" };
  if (verdict === "too_large") return { ok: false, reason: "too_large" };
  if (verdict === "malformed") return { ok: false, reason: "invalid" };
  return { ok: true, contentType: TYPE_OF[kind], ext: EXT_OF[kind], bytes };
}

export async function storeAttachment(r2: R2Bucket, d: Extract<DecodedAttachment, { ok: true }>, index: number): Promise<StoredAttachment> {
  const key = newAttachmentKey();
  await r2.put(ATTACHMENT_PREFIX + key, d.bytes, { httpMetadata: { contentType: d.contentType } });
  return { key, name: `screenshot-${index + 1}.${d.ext}`, contentType: d.contentType, size: d.bytes.length };
}

export async function deleteAttachments(r2: R2Bucket, stored: StoredAttachment[]): Promise<void> {
  if (stored.length > 0) await r2.delete(stored.map((a) => ATTACHMENT_PREFIX + a.key));
}

export function attachmentUrl(origin: string, key: string): string {
  return `${origin}${ATTACHMENT_ROUTE}${key}`;
}

// An image stays private until a maintainer releases it with publishImages, so
// an unreleased, removed or unknown key all answer the same 404.
export async function serveAttachment(env: Env, key: string): Promise<Response> {
  const r2 = env.TELEMETRY_RAW;
  if (!r2 || !/^[A-Za-z0-9_-]{16,64}$/.test(key)) return new Response("not found", { status: 404 });
  if ((await env.DB.prepare("SELECT 1 AS x FROM feedback_public_images WHERE key = ?").bind(key).first()) === null) return new Response("not found", { status: 404 });
  const obj = await r2.get(ATTACHMENT_PREFIX + key);
  const type = obj?.httpMetadata?.contentType;
  if (!obj || (type !== "image/png" && type !== "image/jpeg")) return new Response("not found", { status: 404 });
  return new Response(obj.body, {
    headers: {
      "content-type": type,
      "x-content-type-options": "nosniff",
      "content-disposition": "inline",
      "content-security-policy": "default-src 'none'; sandbox",
      "cache-control": "public, max-age=300",
    },
  });
}

export async function publishAttachments(env: Env, receipt: string, stored: StoredAttachment[]): Promise<void> {
  const at = new Date().toISOString();
  for (const a of stored) await env.DB.prepare("INSERT OR IGNORE INTO feedback_public_images (key, receipt, created_at) VALUES (?,?,?)").bind(a.key, receipt, at).run();
}

// Objects go first: a failed delete leaves the row and the public index untouched
// so the operation can simply be repeated.
export async function withdrawAttachments(env: Env, receipt: string, stored: StoredAttachment[]): Promise<void> {
  if (env.TELEMETRY_RAW) await deleteAttachments(env.TELEMETRY_RAW, stored);
  await env.DB.prepare("DELETE FROM feedback_public_images WHERE receipt = ?").bind(receipt).run();
}
