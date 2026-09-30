import type { Env } from "./env";
import { handleAdmin } from "./feedback_admin";
import { ATTACHMENT_ROUTE, serveAttachment } from "./feedback_attachments";
import { handleMine } from "./feedback_read";
import { handleSubmit } from "./feedback_submit";

// Returns null when the path is not a feedback route so the caller keeps routing.
export async function handleFeedbackRoute(request: Request, env: Env): Promise<Response | null> {
  const url = new URL(request.url);
  const path = url.pathname;
  const method = request.method;
  if (path === "/v1/feedback") return method === "POST" ? handleSubmit(request, env) : new Response("method not allowed", { status: 405 });
  if (path === "/v1/feedback/mine") return method === "GET" ? handleMine(request, env) : new Response("method not allowed", { status: 405 });
  if (path.startsWith(ATTACHMENT_ROUTE)) {
    return method === "GET" ? serveAttachment(env.FEEDBACK_R2, path.slice(ATTACHMENT_ROUTE.length)) : new Response("method not allowed", { status: 405 });
  }
  return handleAdmin(request, env, url);
}
