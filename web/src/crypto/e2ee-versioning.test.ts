/**
 * Unit tests for E2EE versioning: isValidBase64, validateAttachmentVersion.
 */
import { describe, it, expect } from 'vitest'
import { isValidBase64, validateAttachmentVersion, E2EE_ATTACHMENT_VERSION_MIN, E2EE_ATTACHMENT_VERSION_MAX } from './e2ee'

describe('isValidBase64', () => {
  it('accepts valid base64', () => {
    expect(isValidBase64('')).toBe(false)
    expect(isValidBase64('abcd')).toBe(true)
    expect(isValidBase64('SGVsbG8=')).toBe(true)
    expect(isValidBase64('SGVsbG8h')).toBe(true)
    expect(isValidBase64('A-Za-z0-9+/')).toBe(true)
  })

  it('rejects invalid base64', () => {
    expect(isValidBase64('abc')).toBe(false) // length % 4 !== 0
    expect(isValidBase64('ab!d')).toBe(false) // invalid char
    expect(isValidBase64('abc ')).toBe(false) // space invalid
    expect(isValidBase64(null as any)).toBe(false)
    expect(isValidBase64(123 as any)).toBe(false)
  })
})

describe('validateAttachmentVersion', () => {
  it('accepts null/undefined', () => {
    expect(() => validateAttachmentVersion(undefined)).not.toThrow()
    expect(() => validateAttachmentVersion(null as any)).not.toThrow()
  })

  it('accepts valid versions', () => {
    expect(() => validateAttachmentVersion(E2EE_ATTACHMENT_VERSION_MIN)).not.toThrow()
    expect(() => validateAttachmentVersion(E2EE_ATTACHMENT_VERSION_MAX)).not.toThrow()
    expect(() => validateAttachmentVersion(2)).not.toThrow()
  })

  it('rejects unsupported versions', () => {
    expect(() => validateAttachmentVersion(0)).toThrow(/Unsupported attachment version: 0/)
    expect(() => validateAttachmentVersion(99)).toThrow(/Unsupported attachment version: 99/)
    expect(() => validateAttachmentVersion(-1)).toThrow(/Unsupported attachment version: -1/)
    expect(() => validateAttachmentVersion(1.5)).toThrow()
    expect(() => validateAttachmentVersion('1' as any)).toThrow()
  })
})
