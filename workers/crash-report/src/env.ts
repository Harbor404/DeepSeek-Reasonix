export interface RateLimiter {
  limit(opts: { key: string }): Promise<{ success: boolean }>;
}

export interface Env {
  DB: D1Database;
  // Skill/MCP registry database — the folded registry API + moderation console
  // read and write it; the crash tables stay in DB.
  REGISTRY_DB: D1Database;
  RATE_LIMITER: RateLimiter;
  PING_LIMITER: RateLimiter;
  METRICS_LIMITER: RateLimiter;
  WRITE_LIMITER?: RateLimiter;
  // In-app feedback loop. FEEDBACK_ENABLED="false" is the kill switch for
  // submissions. FEEDBACK_TOKEN_SECRET signs install tokens and
  // FEEDBACK_ADMIN_TOKEN gates the converter endpoints; both are worker secrets.
  FEEDBACK_LIMITER?: RateLimiter;
  FEEDBACK_BUDGET_LIMITER?: RateLimiter;
  FEEDBACK_R2?: R2Bucket;
  FEEDBACK_ENABLED?: string;
  FEEDBACK_TOKEN_SECRET?: string;
  FEEDBACK_ADMIN_TOKEN?: string;
  ADMIN_EMAILS?: string;
  // Shared identity service (id.reasonix.io) and the site that hosts its login
  // page (reasonix.io). Overridable for local dev.
  ID_ORIGIN?: string;
  APP_ORIGIN?: string;
  // Browsers allowed to call the registry API with credentials (comma-separated).
  ALLOWED_ORIGINS?: string;
  // Optional incident webhook for the ingest sentinel, mirrored from the
  // GitHub repo secret of the same name by deploy-crash-worker.yml. Feishu/
  // Lark bot URLs get their native payload shape; any other receiver gets
  // Slack-style {"text": ...}. Unset = log-only.
  ALERT_WEBHOOK?: string;
}
