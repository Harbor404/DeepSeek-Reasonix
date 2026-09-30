import type { Env } from "./env";
import { constantTimeEqual, installToken, sha256Hex } from "./feedback_crypto";
import { refuse } from "./feedback_http";

// Endpoints stay off until both secrets exist, so a half-configured deploy never
// mints tokens the converter cannot be authorised against.
export function feedbackEnabled(env: Env): boolean {
  return env.FEEDBACK_ENABLED !== "false" && !!env.FEEDBACK_TOKEN_SECRET && !!env.FEEDBACK_ADMIN_TOKEN;
}

export async function verifyInstall(request: Request, env: Env): Promise<{ installHash: string } | Response> {
  if (!feedbackEnabled(env)) return refuse("feedback.disabled", "feedback is unavailable");
  const id = request.headers.get("x-install-id") ?? "";
  const token = request.headers.get("x-install-token") ?? "";
  if (!env.FEEDBACK_TOKEN_SECRET || !/^[A-Za-z0-9_-]{16,64}$/.test(id) || !token) {
    return refuse("feedback.bad_token", "install token missing or invalid");
  }
  if (!(await constantTimeEqual(token, await installToken(env.FEEDBACK_TOKEN_SECRET, id)))) {
    return refuse("feedback.bad_token", "install token missing or invalid");
  }
  return { installHash: await sha256Hex(id) };
}

export async function requireAdmin(request: Request, env: Env): Promise<Response | null> {
  const header = request.headers.get("authorization") ?? "";
  const presented = header.startsWith("Bearer ") ? header.slice(7) : "";
  if (!env.FEEDBACK_ADMIN_TOKEN || !presented || !(await constantTimeEqual(presented, env.FEEDBACK_ADMIN_TOKEN))) {
    return refuse("feedback.unauthorized", "admin token missing or invalid");
  }
  return null;
}
