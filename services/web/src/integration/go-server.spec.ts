/// <reference types="node" />
import { describe, expect, it } from 'vitest'

import { GO_INTEGRATION_STARTUP_TIMEOUT_MS, buildGoIntegrationServerEnv } from './go-server'

describe('buildGoIntegrationServerEnv', () => {
  it('allows a full minute for a cold Go server startup', () => {
    expect(GO_INTEGRATION_STARTUP_TIMEOUT_MS).toBe(60000)
  })

  it('inherits the caller Go build cache instead of replacing it', () => {
    const sourceEnv: NodeJS.ProcessEnv = {
      GOCACHE: '/home/runner/.cache/go-build',
      PATH: '/usr/local/bin',
    }

    const environment = buildGoIntegrationServerEnv(
      'postgresql://tester:secret@127.0.0.1:5432/ember_integration',
      43123,
      sourceEnv,
    )

    expect(environment).toMatchObject({
      GOCACHE: '/home/runner/.cache/go-build',
      PATH: '/usr/local/bin',
      DATABASE_URL: 'postgresql://tester:secret@127.0.0.1:5432/ember_integration',
      PORT: '43123',
      GIN_MODE: 'test',
      CONFIG_ENCRYPTION_KEY: 'kkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkk',
    })
  })

  it('does not inject an isolated Go build cache when the caller has none', () => {
    const environment = buildGoIntegrationServerEnv(
      'postgresql://tester:secret@127.0.0.1:5432/ember_integration',
      43123,
      { PATH: '/usr/local/bin' },
    )

    expect(environment).not.toHaveProperty('GOCACHE')
  })
})
