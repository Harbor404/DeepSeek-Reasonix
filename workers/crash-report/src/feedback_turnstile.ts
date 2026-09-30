import type { Env } from "./env";

const VERIFY_URL = "https://challenges.cloudflare.com/turnstile/v0/siteverify";
const TIMEOUT_MS = 3000;
export const TURNSTILE_ACTION = "feedback";

// Off unless FEEDBACK_TURNSTILE_SECRET is set. Anything short of a clean, in-time
// success for this action (and hostname, when FEEDBACK_TURNSTILE_HOSTNAMES is set)
// counts as failed: the challenge fails closed.
export async function challengePassed(env: Env, token: string | undefined, ip: string): Promise<boolean> {
  if (!env.FEEDBACK_TURNSTILE_SECRET) return true;
  if (!token) return false;
  try {
    const form = new URLSearchParams({ secret: env.FEEDBACK_TURNSTILE_SECRET, response: token });
    if (ip !== "unknown") form.set("remoteip", ip);
    const res = await fetch(VERIFY_URL, { method: "POST", body: form, signal: AbortSignal.timeout(TIMEOUT_MS) });
    if (!res.ok) return false;
    const out = (await res.json()) as { success?: boolean; action?: string; hostname?: string };
    if (out.success !== true || out.action !== TURNSTILE_ACTION) return false;
    const hosts = (env.FEEDBACK_TURNSTILE_HOSTNAMES ?? "").split(",").map((h) => h.trim()).filter(Boolean);
    return hosts.length === 0 || (out.hostname !== undefined && hosts.includes(out.hostname));
  } catch {
    return false;
  }
}
