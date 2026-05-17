import { describe, expect, it } from 'vitest'

import { normalizeCommitSha, normalizeRepositoryUrl } from './buildInfo.js'

describe('build info utils', () => {
  it('normalizes repository slugs and urls', () => {
    expect(normalizeRepositoryUrl('konghanghang/ember')).toBe('https://github.com/konghanghang/ember')
    expect(normalizeRepositoryUrl('https://github.com/konghanghang/ember/')).toBe('https://github.com/konghanghang/ember')
    expect(normalizeRepositoryUrl(undefined)).toBe('https://github.com/konghanghang/ember')
  })

  it('accepts only commit-like sha values', () => {
    expect(normalizeCommitSha('abcdef1')).toBe('abcdef1')
    expect(normalizeCommitSha('abcdef1234567890abcdef1234567890abcdef12')).toBe('abcdef1234567890abcdef1234567890abcdef12')
    expect(normalizeCommitSha('preview')).toBe('')
    expect(normalizeCommitSha(undefined)).toBe('')
  })
})
