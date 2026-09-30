import { MAX_IMAGE_PIXELS } from "./feedback_types";

export type ImageKind = "png" | "jpeg";
export type ImageVerdict = "clean" | "metadata" | "malformed" | "too_large";

const PNG_SIGNATURE = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];
// Critical chunks plus the colour-rendering ancillaries; everything else (text,
// time, Exif, animation, private chunks) can carry data the user never meant to send.
const PNG_ALLOWED = new Set(["IHDR", "PLTE", "IDAT", "IEND", "gAMA", "cHRM", "sRGB", "iCCP", "pHYs", "tRNS", "bKGD", "sBIT"]);
// JFIF, ICC profile and Adobe colour transform are the only application segments kept.
const JPEG_APP_ALLOWED = new Set([0xe0, 0xe2, 0xee]);
const JPEG_STRUCTURAL = new Set([0xdb, 0xc4, 0xdd, 0xda, 0xcc]);

export function sniffImage(b: Uint8Array): ImageKind | null {
  if (b.length >= 8 && PNG_SIGNATURE.every((m, i) => b[i] === m)) return "png";
  if (b.length >= 3 && b[0] === 0xff && b[1] === 0xd8 && b[2] === 0xff) return "jpeg";
  return null;
}

function pixelVerdict(w: number, h: number): ImageVerdict {
  if (w === 0 || h === 0) return "malformed";
  return w * h > MAX_IMAGE_PIXELS ? "too_large" : "clean";
}

function checkPng(b: Uint8Array): ImageVerdict {
  const view = new DataView(b.buffer, b.byteOffset, b.byteLength);
  let at = 8;
  let dims: ImageVerdict | null = null;
  let sawData = false;
  while (at + 12 <= b.length) {
    const len = view.getUint32(at);
    const type = String.fromCharCode(b[at + 4], b[at + 5], b[at + 6], b[at + 7]);
    const end = at + 12 + len;
    if (end > b.length) return "malformed";
    if (at === 8) {
      if (type !== "IHDR" || len !== 13) return "malformed";
      dims = pixelVerdict(view.getUint32(at + 8), view.getUint32(at + 12));
    }
    if (!PNG_ALLOWED.has(type)) return "metadata";
    if (type === "IDAT") sawData = true;
    if (type === "IEND") {
      if (dims !== "clean") return dims ?? "malformed";
      if (!sawData) return "malformed";
      return end === b.length ? "clean" : "metadata";
    }
    at = end;
  }
  return "malformed";
}

function isStartOfFrame(marker: number): boolean {
  return marker >= 0xc0 && marker <= 0xcf && marker !== 0xc4 && marker !== 0xc8 && marker !== 0xcc;
}

function checkJpeg(b: Uint8Array): ImageVerdict {
  let at = 2;
  let dims: ImageVerdict | null = null;
  while (at + 2 <= b.length) {
    if (b[at] !== 0xff) return "malformed";
    const marker = b[at + 1];
    if (marker === 0xff) {
      at += 1;
      continue;
    }
    if (marker === 0xd9) {
      if (dims !== "clean") return dims ?? "malformed";
      return at + 2 === b.length ? "clean" : "metadata";
    }
    if (marker === 0x00 || (marker >= 0xd0 && marker <= 0xd8)) {
      at += 2;
      continue;
    }
    if (at + 4 > b.length) return "malformed";
    const len = (b[at + 2] << 8) | b[at + 3];
    if (len < 2 || at + 2 + len > b.length) return "malformed";
    if (isStartOfFrame(marker)) {
      if (len < 8) return "malformed";
      dims = pixelVerdict((b[at + 7] << 8) | b[at + 8], (b[at + 5] << 8) | b[at + 6]);
    } else if (marker >= 0xe0 && marker <= 0xef ? !JPEG_APP_ALLOWED.has(marker) : !JPEG_STRUCTURAL.has(marker)) {
      return "metadata";
    }
    at += 2 + len;
    if (marker === 0xda) {
      // Entropy-coded data runs to the next marker that is not a stuffed 0xFF or a restart.
      while (at + 1 < b.length && !(b[at] === 0xff && b[at + 1] !== 0x00 && !(b[at + 1] >= 0xd0 && b[at + 1] <= 0xd7))) at++;
    }
  }
  return "malformed";
}

// Structure is walked, never decoded, so a hostile file costs one linear pass.
export function inspectImage(b: Uint8Array, kind: ImageKind): ImageVerdict {
  return kind === "png" ? checkPng(b) : checkJpeg(b);
}
