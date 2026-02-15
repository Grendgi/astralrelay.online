/**
 * Lazy-load QR generation for Safety Number display.
 * Encodes the safety number string for out-of-band verification (scan and compare).
 */
export async function safetyNumberToQRDataUrl(safetyNumber: string, size = 180): Promise<string> {
  const QRCode = (await import('qrcode')).default
  return QRCode.toDataURL(safetyNumber, { width: size, margin: 1 })
}
