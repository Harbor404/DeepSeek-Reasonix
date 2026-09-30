import type { Env } from "./env";

// Atomically takes one unit from a fixed-window counter; false once `limit` is reached.
async function take(env: Env, bucket: string, day: string, limit: number): Promise<boolean> {
  const row = await env.DB.prepare(
    "INSERT INTO feedback_quota (bucket, n, day) VALUES (?, 1, ?) ON CONFLICT (bucket) DO UPDATE SET n = n + 1 WHERE n < ? RETURNING n",
  )
    .bind(bucket, day, limit)
    .first<{ n: number }>();
  return row !== null;
}

export interface QuotaKeys {
  ipKey: string;
  installHash: string;
}

export interface QuotaLimits {
  globalDaily: number;
  ipHourly: number;
  installHourly: number;
  installDaily: number;
}

export type Admission = "ok" | "limited" | "busy";

// The caller's own limits are spent before the shared daily budget, and a refusal
// hands back every unit already taken, so a rejected request costs nothing.
export async function admit(env: Env, keys: QuotaKeys, now: Date, limits: QuotaLimits): Promise<Admission> {
  const day = now.toISOString().slice(0, 10);
  const hour = now.toISOString().slice(0, 13);
  const steps: [string, number][] = [
    [`ip:${keys.ipKey}:${hour}`, limits.ipHourly],
    [`ih:${keys.installHash}:${hour}`, limits.installHourly],
    [`id:${keys.installHash}:${day}`, limits.installDaily],
    [`g:${day}`, limits.globalDaily],
  ];
  const taken: string[] = [];
  for (const [bucket, limit] of steps) {
    if (!(await take(env, bucket, day, limit))) {
      for (const b of taken) await env.DB.prepare("UPDATE feedback_quota SET n = n - 1 WHERE bucket = ? AND n > 0").bind(b).run();
      return bucket.startsWith("g:") ? "busy" : "limited";
    }
    taken.push(bucket);
  }
  return "ok";
}

// True only for the first caller of the day, so an exhausted budget alerts once.
export async function firstBusyOfDay(env: Env, now: Date): Promise<boolean> {
  const day = now.toISOString().slice(0, 10);
  return take(env, `alert:${day}`, day, 1);
}
