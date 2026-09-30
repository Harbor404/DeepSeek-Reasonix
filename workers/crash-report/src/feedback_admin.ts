import type { Env } from "./env";
import { requireAdmin } from "./feedback_auth";
import { jsonResponse, refuse } from "./feedback_http";
import { pendingItem } from "./feedback_read";
import { RecordedBody, StatusBody } from "./feedback_schema";
import { statusRank, type FeedbackRow, type Status } from "./feedback_types";

const PENDING_DEFAULT = 20;
const PENDING_MAX = 50;
const OPEN_LIMIT = 200;

async function load(env: Env, receipt: string): Promise<FeedbackRow | null> {
  return env.DB.prepare("SELECT * FROM feedback WHERE receipt = ?").bind(receipt).first<FeedbackRow>();
}

async function readJson(request: Request): Promise<unknown> {
  try {
    return await request.json();
  } catch {
    return undefined;
  }
}

async function setState(env: Env, receipt: string, from: Status, sets: string, binds: unknown[]): Promise<boolean> {
  const res = await env.DB.prepare(`UPDATE feedback SET ${sets}, updated_at = ? WHERE receipt = ? AND status = ?`)
    .bind(...binds, new Date().toISOString(), receipt, from)
    .run();
  return (res.meta?.changes ?? 0) > 0;
}

async function pending(request: Request, env: Env, url: URL): Promise<Response> {
  const asked = Number(url.searchParams.get("limit") ?? PENDING_DEFAULT);
  const limit = Math.min(PENDING_MAX, Math.max(1, Number.isFinite(asked) ? Math.trunc(asked) : PENDING_DEFAULT));
  const { results } = await env.DB.prepare("SELECT * FROM feedback WHERE status = 'received' ORDER BY created_at ASC LIMIT ?")
    .bind(limit)
    .all<FeedbackRow>();
  return jsonResponse({ items: results.map((r) => pendingItem(r, url.origin)) });
}

async function open(env: Env): Promise<Response> {
  const { results } = await env.DB.prepare(
    "SELECT receipt, status, issue_number, issue_url FROM feedback WHERE status IN ('recorded','in_progress') ORDER BY created_at ASC LIMIT ?",
  )
    .bind(OPEN_LIMIT)
    .all<Pick<FeedbackRow, "receipt" | "status" | "issue_number" | "issue_url">>();
  return jsonResponse({
    items: results.map((r) => ({ receipt: r.receipt, status: r.status, issueNumber: r.issue_number, issueUrl: r.issue_url })),
  });
}

async function recorded(request: Request, env: Env, receipt: string): Promise<Response> {
  const body = RecordedBody.safeParse(await readJson(request));
  if (!body.success) return refuse("feedback.invalid", "issueNumber and issueUrl are required");
  const row = await load(env, receipt);
  if (!row) return refuse("feedback.not_found", "unknown receipt");
  if (statusRank(row.status) >= 1) return jsonResponse({ receipt, status: row.status });
  if (row.status !== "received") return refuse("feedback.bad_transition", "feedback is not releasable to the converter");
  const ok = await setState(env, receipt, "received", "status = 'recorded', issue_number = ?, issue_url = ?", [body.data.issueNumber, body.data.issueUrl]);
  if (!ok) return refuse("feedback.bad_transition", "status changed concurrently");
  return jsonResponse({ receipt, status: "recorded" });
}

async function status(request: Request, env: Env, receipt: string): Promise<Response> {
  const body = StatusBody.safeParse(await readJson(request));
  if (!body.success) return refuse("feedback.invalid", "unknown or malformed status update");
  const { status: next, resolvedVersion, duplicateOf } = body.data;
  if (next === "fixed" && !resolvedVersion) return refuse("feedback.invalid", "fixed requires resolvedVersion or \"next\"");
  const row = await load(env, receipt);
  if (!row) return refuse("feedback.not_found", "unknown receipt");
  if (row.status === next) return jsonResponse({ receipt, status: row.status });
  if (row.status === "received" || row.status === "held" || statusRank(next) <= statusRank(row.status)) {
    return refuse("feedback.bad_transition", "only forward transitions from a recorded report are allowed");
  }
  const ok = await setState(env, receipt, row.status, "status = ?, resolved_version = ?, duplicate_of = ?", [
    next,
    next === "fixed" ? (resolvedVersion ?? null) : null,
    next === "duplicate" ? (duplicateOf ?? null) : null,
  ]);
  if (!ok) return refuse("feedback.bad_transition", "status changed concurrently");
  return jsonResponse({ receipt, status: next });
}

async function release(env: Env, receipt: string): Promise<Response> {
  const row = await load(env, receipt);
  if (!row) return refuse("feedback.not_found", "unknown receipt");
  if (row.status === "received") return jsonResponse({ receipt, status: "received" });
  if (row.status !== "held") return refuse("feedback.bad_transition", "only held feedback can be released");
  if (!(await setState(env, receipt, "held", "status = 'received'", []))) return refuse("feedback.bad_transition", "status changed concurrently");
  return jsonResponse({ receipt, status: "received" });
}

export async function handleAdmin(request: Request, env: Env, url: URL): Promise<Response | null> {
  const path = url.pathname;
  const method = request.method;
  const isPending = path === "/v1/admin/feedback/pending";
  const isOpen = path === "/v1/admin/feedback/open";
  const m = path.match(/^\/v1\/admin\/feedback\/(FB-[0-9A-Z]{4}-[0-9A-Z]{4})\/(recorded|status|release)$/);
  if (!isPending && !isOpen && !m) return null;
  const denied = await requireAdmin(request, env);
  if (denied) return denied;
  if (isPending && method === "GET") return pending(request, env, url);
  if (isOpen && method === "GET") return open(env);
  if (m && method === "POST") {
    if (m[2] === "recorded") return recorded(request, env, m[1]);
    if (m[2] === "status") return status(request, env, m[1]);
    return release(env, m[1]);
  }
  return new Response("method not allowed", { status: 405 });
}
