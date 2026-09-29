import type { Bindings } from "./env";

export async function remoteDevicePresence(
  env: Bindings,
  deviceIds: string[],
): Promise<{ onlineIds: Set<string>; available: boolean }> {
  if (deviceIds.length === 0) return { onlineIds: new Set(), available: true };
  if (!env.REMOTE_GATEWAY_TOKEN || !env.REMOTE_GATEWAY_ORIGIN) {
    return { onlineIds: new Set(), available: false };
  }
  try {
    const response = await fetch(`${env.REMOTE_GATEWAY_ORIGIN.replace(/\/+$/, "")}/v1/devices/status`, {
      method: "POST",
      headers: {
        "content-type": "application/json",
        "x-reasonix-gateway-token": env.REMOTE_GATEWAY_TOKEN,
      },
      body: JSON.stringify({ deviceIds: deviceIds.slice(0, 50) }),
    });
    if (!response.ok) return { onlineIds: new Set(), available: false };
    const data = await response.json<{ devices?: Array<{ id?: string; online?: boolean }> }>();
    const onlineIds = new Set(
      (data.devices ?? []).filter((device) => device.online === true && typeof device.id === "string")
        .map((device) => device.id as string),
    );
    return { onlineIds, available: true };
  } catch {
    return { onlineIds: new Set(), available: false };
  }
}

// Asks the relay to close live connections the account service has just
// revoked. With `sessionId`, only controllers admitted through that account
// session close; without it, every connection to the named devices does.
// The relay also re-checks each connection on a timer, so a failed call here
// delays the disconnect rather than losing it.
export async function disconnectRemote(
  env: Bindings,
  deviceIds: string[],
  sessionId?: string,
): Promise<boolean> {
  if (deviceIds.length === 0) return true;
  if (!env.REMOTE_GATEWAY_TOKEN || !env.REMOTE_GATEWAY_ORIGIN) return false;
  const origin = env.REMOTE_GATEWAY_ORIGIN.replace(/\/+$/, "");
  let delivered = true;
  for (let at = 0; at < deviceIds.length; at += 50) {
    const batch = deviceIds.slice(at, at + 50);
    const body = JSON.stringify(sessionId ? { deviceIds: batch, sessionId } : { deviceIds: batch });
    let ok = false;
    for (let attempt = 0; attempt < 2 && !ok; attempt += 1) {
      try {
        const response = await fetch(`${origin}/v1/devices/revoke`, {
          method: "POST",
          headers: {
            "content-type": "application/json",
            "x-reasonix-gateway-token": env.REMOTE_GATEWAY_TOKEN,
          },
          body,
        });
        ok = response.ok;
      } catch {
        ok = false;
      }
    }
    delivered &&= ok;
  }
  return delivered;
}
