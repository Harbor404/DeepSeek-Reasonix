import { z } from "zod";
import { CATEGORIES } from "./feedback_types";

const short = (n: number) => z.string().max(n).optional();

export const FeedbackEnv = z.object({
  version: short(40),
  commit: short(40),
  surface: z.enum(["studio", "tui", "acp"]).optional(),
  os: short(40),
  osVersion: short(80),
  arch: short(20),
  locale: short(20),
  channel: z.enum(["stable", "preview"]).optional(),
  providerKind: z.enum(["deepseek", "openai", "anthropic", "other"]).optional(),
});

export const FeedbackSubmit = z.object({
  idempotencyKey: z.string().min(8).max(64),
  installId: z.string().regex(/^[A-Za-z0-9_-]{16,64}$/),
  category: z.enum(CATEGORIES),
  body: z.string().refine((s) => s.trim().length > 0),
  displayName: z.string().trim().min(1).max(40),
  contact: z.string().max(120).optional(),
  env: FeedbackEnv.default({}),
  attachments: z
    .array(z.object({ name: z.string().max(200), contentType: z.string().max(40), dataBase64: z.string() }))
    .max(3)
    .default([]),
});
export type FeedbackSubmitInput = z.infer<typeof FeedbackSubmit>;

export const RecordedBody = z.object({ issueNumber: z.number().int().positive(), issueUrl: z.string().url().max(300) });

export const StatusBody = z.object({
  status: z.enum(["in_progress", "fixed", "wontfix", "duplicate"]),
  resolvedVersion: z.string().max(40).optional(),
  duplicateOf: z.string().max(40).optional(),
});
