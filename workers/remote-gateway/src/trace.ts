export type TraceValue = string | number | boolean | null | undefined;

const WINDOW_MS = 60 * 1000;
const BURST = 20;

interface Bucket {
  start: number;
  count: number;
  dropped: number;
}

const buckets = new Map<string, Bucket>();

// Unsalted 32-bit FNV: it only groups lines of one device or grant. Reading an
// id back out of it relies on those ids and tokens being high-entropy secrets.
export function coarseHash(value: string): string {
  let hash = 0x811c9dc5;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193);
  }
  return (hash >>> 0).toString(16).padStart(8, "0");
}

// One JSON line per event. Each event+code pair may log BURST lines a minute
// per isolate, so this is not a global cap; the next window's first line
// reports how many were skipped. Logging never throws into the caller.
export function trace(event: string, fields: Record<string, TraceValue>, now = Date.now()): void {
  try {
    emit(event, fields, now);
  } catch {
    // A failed log line must not fail the connection it describes.
  }
}

function emit(event: string, fields: Record<string, TraceValue>, now: number): void {
  const key = `${event}:${String(fields.code ?? "")}`;
  let bucket = buckets.get(key);
  let suppressed = 0;
  if (!bucket || now - bucket.start >= WINDOW_MS) {
    suppressed = bucket?.dropped ?? 0;
    bucket = { start: now, count: 0, dropped: 0 };
    buckets.set(key, bucket);
  }
  if (bucket.count >= BURST) {
    bucket.dropped += 1;
    return;
  }
  bucket.count += 1;
  console.log(JSON.stringify({ svc: "remote-gateway", event, ...fields, ...(suppressed > 0 ? { suppressed } : {}) }));
}

export function resetTrace(): void {
  buckets.clear();
}
