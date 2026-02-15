/**
 * Safe logging: avoid leaking plaintext, tokens, or sensitive data.
 * Use instead of console.error when errors might contain user data or URLs with tokens.
 */
export function logError(context: string, err?: unknown): void {
  const msg = err instanceof Error ? err.message : String(err ?? 'Unknown')
  // Avoid logging full error (stack may contain URLs, response bodies)
  if (typeof console !== 'undefined' && console.error) {
    console.error(`[${context}]`, msg)
  }
}
