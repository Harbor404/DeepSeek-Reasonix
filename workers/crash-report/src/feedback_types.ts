export const CATEGORIES = ["bug", "idea", "question", "other"] as const;
export type Category = (typeof CATEGORIES)[number];

export const STATUSES = ["received", "held", "recorded", "in_progress", "fixed", "wontfix", "duplicate"] as const;
export type Status = (typeof STATUSES)[number];

export const MAX_BODY_BYTES = 8192;
export const MAX_ATTACHMENT_BYTES = 2 * 1024 * 1024;
export const MAX_ATTACHMENTS = 3;
export const MAX_REQUEST_BYTES = 8 * 1024 * 1024;
export const PER_INSTALL_HOURLY = 3;
export const PER_IP_HOURLY = 10;
export const GLOBAL_DAILY = 300;
export const UNCONVERTED_RETENTION_DAYS = 30;
export const MAX_IMAGE_PIXELS = 40_000_000;
export const PER_INSTALL_DAILY = 10;

export interface StoredAttachment {
  key: string;
  name: string;
  contentType: string;
  size: number;
}

export interface FeedbackRow {
  receipt: string;
  install_hash: string;
  category: Category;
  body: string;
  display_name: string;
  contact: string;
  env_json: string;
  attachments_json: string;
  status: Status;
  issue_number: number | null;
  issue_url: string | null;
  resolved_version: string | null;
  duplicate_of: string | null;
  created_at: string;
  updated_at: string;
}

// Rank orders the forward-only lifecycle; the three outcomes share the top rank
// so none of them can move to another.
export function statusRank(s: Status): number {
  switch (s) {
    case "held":
    case "received":
      return 0;
    case "recorded":
      return 1;
    case "in_progress":
      return 2;
    default:
      return 3;
  }
}
