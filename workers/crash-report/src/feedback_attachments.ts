import { newAttachmentKey } from "./feedback_crypto";
import { MAX_ATTACHMENT_BYTES, type StoredAttachment } from "./feedback_types";

export const ATTACHMENT_PREFIX = "feedback/";
export const ATTACHMENT_ROUTE = "/v1/feedback/attachments/";

const PNG = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];
const JPEG = [0xff, 0xd8, 0xff];
const MAGIC: Record<string, number[]> = { "image/png": PNG, "image/jpeg": JPEG };

export interface AttachmentInput {
  name: string;
  contentType: string;
  dataBase64: string;
}

export type DecodedAttachment = { ok: true; name: string; contentType: string; bytes: Uint8Array } | { ok: false; reason: "too_large" | "invalid" };

export function decodeAttachment(a: AttachmentInput): DecodedAttachment {
  const magic = MAGIC[a.contentType];
  if (!magic) return { ok: false, reason: "invalid" };
  if (a.dataBase64.length > Math.ceil((MAX_ATTACHMENT_BYTES * 4) / 3) + 4) return { ok: false, reason: "too_large" };
  let bytes: Uint8Array;
  try {
    const bin = atob(a.dataBase64);
    bytes = Uint8Array.from(bin, (c) => c.charCodeAt(0));
  } catch {
    return { ok: false, reason: "invalid" };
  }
  if (bytes.length > MAX_ATTACHMENT_BYTES) return { ok: false, reason: "too_large" };
  if (bytes.length < magic.length || !magic.every((m, i) => bytes[i] === m)) return { ok: false, reason: "invalid" };
  const name = a.name.replace(/[^\w.\- ]/g, "_").slice(0, 80) || "image";
  return { ok: true, name, contentType: a.contentType, bytes };
}

export async function storeAttachment(r2: R2Bucket, d: Extract<DecodedAttachment, { ok: true }>): Promise<StoredAttachment> {
  const key = newAttachmentKey();
  await r2.put(ATTACHMENT_PREFIX + key, d.bytes, { httpMetadata: { contentType: d.contentType } });
  return { key, name: d.name, contentType: d.contentType, size: d.bytes.length };
}

export function attachmentUrl(origin: string, key: string): string {
  return `${origin}${ATTACHMENT_ROUTE}${key}`;
}

export async function serveAttachment(r2: R2Bucket | undefined, key: string): Promise<Response> {
  if (!r2 || !/^[A-Za-z0-9_-]{16,64}$/.test(key)) return new Response("not found", { status: 404 });
  const obj = await r2.get(ATTACHMENT_PREFIX + key);
  const type = obj?.httpMetadata?.contentType;
  if (!obj || !type || !MAGIC[type]) return new Response("not found", { status: 404 });
  return new Response(obj.body, {
    headers: {
      "content-type": type,
      "x-content-type-options": "nosniff",
      "content-disposition": "inline",
      "cache-control": "public, max-age=31536000, immutable",
    },
  });
}
