import type { Env } from "./env";

export async function sendAlert(env: Env, text: string): Promise<void> {
  if (!env.ALERT_WEBHOOK) return;
  try {
    const webhook = new URL(env.ALERT_WEBHOOK);
    const feishu = webhook.hostname === "open.feishu.cn" || webhook.hostname === "open.larksuite.com";
    const body = feishu ? { msg_type: "text", content: { text } } : { text };
    const res = await fetch(webhook.toString(), {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!res.ok) console.error(`alert webhook responded ${res.status}`);
  } catch (err) {
    console.error("alert webhook unreachable", err);
  }
}
