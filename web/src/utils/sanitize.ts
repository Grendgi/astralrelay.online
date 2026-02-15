/**
 * Content sanitization for safe display.
 * React escapes by default when rendering {text} in JSX.
 * This helper validates input; use DOMPurify only if rendering HTML via dangerouslySetInnerHTML.
 */
export function sanitizeForDisplay(text: string): string {
  if (typeof text !== 'string') return ''
  return text
}
