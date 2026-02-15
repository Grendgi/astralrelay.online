/**
 * Content sanitization for safe display.
 * React escapes by default when rendering {text} in JSX.
 * This helper validates input; use DOMPurify only if rendering HTML via dangerouslySetInnerHTML.
 */
export function sanitizeForDisplay(text: string): string {
  if (typeof text !== 'string') return ''
  return text
}

/** Sanitize filename for E2EE payload: basename only, no path, limit length, remove null/path chars. */
export function sanitizeFilename(name: string): string {
  if (typeof name !== 'string') return 'file'
  const MAX = 255
  let s = name.replace(/\0/g, '').replace(/[/\\]/g, '')
  const last = s.lastIndexOf('/')
  if (last >= 0) s = s.slice(last + 1)
  if (!s.trim()) return 'file'
  return s.trim().slice(0, MAX) || 'file'
}
