const RECEIPT = "FB-[0-9A-Z]{4}-[0-9A-Z]{4}";

// The only calls scripts/feedback-sync.mjs makes; the exact method+path shapes are the whole allow-list.
const CONVERTER_ROUTES: readonly { method: string; path: RegExp }[] = [
  { method: "GET", path: /^\/v1\/admin\/feedback\/pending$/ },
  { method: "GET", path: /^\/v1\/admin\/feedback\/open$/ },
  { method: "GET", path: /^\/v1\/admin\/feedback\/replies\/pending$/ },
  { method: "POST", path: new RegExp(`^/v1/admin/feedback/${RECEIPT}/recorded$`) },
  { method: "POST", path: new RegExp(`^/v1/admin/feedback/${RECEIPT}/status$`) },
  { method: "POST", path: /^\/v1\/admin\/feedback\/replies\/[A-Za-z0-9_-]{1,64}\/ack$/ },
];

export function isWorkersDevHost(hostname: string): boolean {
  return hostname.replace(/\.$/, "").endsWith(".workers.dev");
}

// Returns a 404 for anything the converter does not need when served from workers.dev, null otherwise.
export function workersDevGate(request: Request): Response | null {
  const url = new URL(request.url);
  if (!isWorkersDevHost(url.hostname)) return null;
  const allowed = CONVERTER_ROUTES.some((r) => r.method === request.method && r.path.test(url.pathname));
  return allowed ? null : new Response("not found", { status: 404 });
}
