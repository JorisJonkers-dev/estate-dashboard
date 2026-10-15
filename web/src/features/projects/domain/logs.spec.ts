import { describe, expect, it } from 'vitest'
import { groupStreams, isMigration, streamKey } from './logs'

const s = (pod: string, container: string) => ({ pod, container })

describe('the log streams of a Project', () => {
  it('puts migration Jobs first, by pod or container name', () => {
    const groups = groupStreams([s('auth-api-5c7e-x', 'auth-api'), s('auth-migration-up-q2', 'liquibase'), s('auth-down-1', 'auth-migrate')])
    expect(groups.map((g) => [g.name, g.streams.map(streamKey)])).toEqual([
      ['Migrations', ['auth-migration-up-q2/liquibase', 'auth-down-1/auth-migrate']],
      ['Containers', ['auth-api-5c7e-x/auth-api']],
    ])
  })

  it('leaves out a group with nothing in it', () => {
    expect(groupStreams([s('web-1', 'web')]).map((g) => g.name)).toEqual(['Containers'])
    expect(groupStreams([])).toEqual([])
    expect(isMigration(s('web-1', 'web'))).toBe(false)
  })
})
