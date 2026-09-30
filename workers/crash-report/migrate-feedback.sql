CREATE TABLE IF NOT EXISTS feedback (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  receipt TEXT NOT NULL UNIQUE,
  install_hash TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  category TEXT NOT NULL,
  body TEXT NOT NULL,
  display_name TEXT NOT NULL,
  contact TEXT NOT NULL DEFAULT '',
  env_json TEXT NOT NULL DEFAULT '{}',
  attachments_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL DEFAULT 'received',
  issue_number INTEGER,
  issue_url TEXT,
  resolved_version TEXT,
  duplicate_of TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (install_hash, idempotency_key)
);

CREATE INDEX IF NOT EXISTS feedback_status_created ON feedback (status, created_at);
CREATE INDEX IF NOT EXISTS feedback_install_created ON feedback (install_hash, created_at);
