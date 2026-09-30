export type FeedbackCode =
  | "feedback.too_large"
  | "feedback.rate_limited"
  | "feedback.invalid"
  | "feedback.disabled"
  | "feedback.duplicate"
  | "feedback.bad_token"
  | "feedback.unauthorized"
  | "feedback.not_found"
  | "feedback.bad_transition"
  | "feedback.method_not_allowed"
  | "feedback.busy"
  | "feedback.image_metadata"
  | "feedback.reply_limit"
  | "feedback.not_replyable"
  | "feedback.challenge_required";

const STATUS: Record<FeedbackCode, number> = {
  "feedback.too_large": 413,
  "feedback.rate_limited": 429,
  "feedback.invalid": 400,
  "feedback.disabled": 503,
  "feedback.duplicate": 409,
  "feedback.bad_token": 401,
  "feedback.unauthorized": 401,
  "feedback.not_found": 404,
  "feedback.bad_transition": 409,
  "feedback.method_not_allowed": 405,
  "feedback.busy": 503,
  "feedback.image_metadata": 400,
  "feedback.reply_limit": 429,
  "feedback.not_replyable": 409,
  "feedback.challenge_required": 403,
};

export function jsonResponse(body: unknown, status = 200, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json; charset=utf-8", "cache-control": "no-store", ...headers },
  });
}

export function refuse(code: FeedbackCode, message: string): Response {
  return jsonResponse({ error: { code, message } }, STATUS[code]);
}
