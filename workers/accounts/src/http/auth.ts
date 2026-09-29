import type { Context, MiddlewareHandler } from "hono";
import type { AppEnv } from "../env";
import type { AccountUser } from "../types";
import { toAccountUser } from "../types";
import { repos } from "../db";
import type { SessionRef } from "../db/sessions";
import { readSessionToken } from "../auth/cookies";
import { ApiError } from "./errors";

// Non-browser clients (CLI/desktop) and cross-service callers carry the session
// in an Authorization header instead of the cookie.
export function readBearerToken(c: Context<AppEnv>): string | undefined {
  const header = c.req.header("authorization");
  if (!header) return undefined;
  const token = /^Bearer\s+(.+)$/i.exec(header.trim())?.[1]?.trim();
  return token || undefined;
}

// Resolves the session (cookie or Bearer token, if any) and stashes the user on
// the context. Runs for every request; never rejects.
export const loadUser: MiddlewareHandler<AppEnv> = async (c, next) => {
  const token = readSessionToken(c) ?? readBearerToken(c);
  let user: AccountUser | null = null;
  let session: SessionRef | null = null;
  if (token) {
    const resolved = await repos(c.env).sessions.resolveSession(token);
    if (resolved) {
      user = toAccountUser(resolved.user);
      session = resolved.session;
    }
  }
  c.set("user", user);
  c.set("session", session);
  await next();
};

// Gate for protected routes. Pairs with currentUser() in handlers.
export const requireAuth: MiddlewareHandler<AppEnv> = async (c, next) => {
  if (!c.get("user")) throw new ApiError(401, "unauthorized", "Sign in to continue.");
  await next();
};

export function currentUser(c: Context<AppEnv>): AccountUser {
  const user = c.get("user");
  if (!user) throw new ApiError(401, "unauthorized", "Sign in to continue.");
  return user;
}
