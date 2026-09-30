// Platform a Studio build can be picked for. `os` is "win" | "mac" | "linux"
// only when a shipped build matches; otherwise it is "" and `reason` says why
// (the page then shows every build with a note instead of recommending one).
// Shipped builds: macOS arm64/amd64, Windows x64 (also runs on Windows ARM
// through emulation), Linux amd64.
export function detectPlatform(ua, maxTouchPoints = 0) {
  const value = typeof ua === "string" ? ua : "";
  if (/Android|iPhone|iPad|iPod|Mobile/i.test(value)) return { os: "", reason: "mobile" };
  if (/Windows/i.test(value)) return { os: "win", reason: "" };
  if (/Macintosh|Mac OS X/i.test(value)) {
    // iPadOS 13+ Safari reports a Macintosh UA; only it has a touch screen.
    return maxTouchPoints > 1 ? { os: "", reason: "mobile" } : { os: "mac", reason: "" };
  }
  if (/CrOS/i.test(value)) return { os: "", reason: "chromeos" };
  if (/Linux|X11/i.test(value)) {
    return /aarch64|arm64|armv\d|\barm\b/i.test(value)
      ? { os: "", reason: "arm-linux" }
      : { os: "linux", reason: "" };
  }
  return { os: "", reason: "unknown" };
}

export function detectDesktopOS(ua, maxTouchPoints = 0) {
  return detectPlatform(ua, maxTouchPoints).os;
}
