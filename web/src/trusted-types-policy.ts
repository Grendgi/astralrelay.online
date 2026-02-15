/**
 * Trusted Types policy for CSP require-trusted-types-for 'script'.
 * Must run before React renders (before any innerHTML sink).
 *
 * The default policy is used by React internals (SVG, script), Vite HMR,
 * and any code that assigns strings to innerHTML/script.src.
 * Pass-through for compatibility; app does not use dangerouslySetInnerHTML (per 7.3).
 * For future HTML rendering, add a separate policy with DOMPurify sanitization.
 */
export function initTrustedTypesPolicy(): void {
  if (typeof window === 'undefined') return
  const tt = (window as unknown as {
    trustedTypes?: {
      createPolicy: (
        name: string,
        opts: { createHTML?: (s: string) => string; createScriptURL?: (s: string) => string }
      ) => unknown
    }
  }).trustedTypes
  if (!tt?.createPolicy) return
  try {
    tt.createPolicy('default', {
      createHTML: (s: string) => s,
      createScriptURL: (s: string) => s,
    })
  } catch {
    // Policy already exists (e.g. duplicate load)
  }
}
