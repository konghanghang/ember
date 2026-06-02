import { describe, expect, it } from 'vitest'

import { formatMediaLibrarySummary } from './media-library'

describe('media library utils', () => {
  it('formats item count only when the API returns it', () => {
    expect(formatMediaLibrarySummary({ type: 'Movie', itemCount: 12 })).toBe('Movie · 12 项')
    expect(formatMediaLibrarySummary({ type: 'Movie' })).toBe('Movie')
    expect(formatMediaLibrarySummary({ type: '', itemCount: 0 })).toBe('Unknown · 0 项')
  })
})
