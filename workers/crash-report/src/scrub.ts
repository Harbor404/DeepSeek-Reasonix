export function scrubSensitiveText(input: string): string {
  return input
    .replace(/([A-Z]:\\Users\\)[^/\\:\s"']+/gi, "$1_")
    .replace(/(\/(?:home|Users)\/)[^/\\:\s"']+/g, "$1_")
    .replace(/\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b/g, "[redacted-email]")
    .replace(/\bBearer\s+[A-Za-z0-9._~+/=-]{16,}/gi, "Bearer [redacted]")
    .replace(
      /\b(api[_-]?key|access[_-]?token|refresh[_-]?token|id[_-]?token|authorization|secret|password|passwd|pwd|token)\b\s*[:=]\s*(?:Bearer\s+)?['"]?[^'"\s,;]+['"]?/gi,
      "$1=[redacted]",
    )
    .replace(/\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b/g, "[redacted-jwt]")
    .replace(/\b(?:sk|rk)-(?:proj-)?[A-Za-z0-9_-]{16,}\b/g, "[redacted-key]")
    .replace(/\b[0-9a-fA-F]{32,}\b/g, "[redacted-hex]")
    .replace(/[A-Za-z0-9+/]{40,}={0,2}/g, "[redacted-token]")
    .replace(/\b[A-Za-z0-9_-]{48,}\b/g, "[redacted-token]");
}
