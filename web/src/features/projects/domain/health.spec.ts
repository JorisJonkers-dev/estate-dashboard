import { describe, expect, it } from 'vitest'
import type { Check } from '../../../infrastructure/api/types.gen'
import { checkHealth, checksOf } from './health'

const now = Date.parse('2026-10-19T21:00:00Z')

const check = (over: Partial<Check> = {}): Check => ({
  key: 'home-media_radarr',
  name: 'radarr',
  group: 'home-media',
  host: 'radarr.arr-system.svc.cluster.local',
  at: '2026-10-19T20:59:30Z',
  up: true,
  status: 200,
  durationMs: 206,
  conditions: [],
  recent: [true, true],
  ...over,
})

describe("a Project's checks", () => {
  it('are the ones that probe its namespace, its name or one of its Processes', () => {
    const all = [
      check(),
      check({ key: 'public-apps_home-portal', name: 'home-portal', host: 'jorisjonkers.dev' }),
      check({
        key: 'public-apps_estate',
        name: 'estate',
        host: 'estate-dashboard.estate-dashboard-system.svc.cluster.local',
      }),
      check({ key: 'home-media_jellyfin', name: 'jellyfin', host: 'jellyfin.jorisjonkers.dev' }),
      check({ key: 'x_sonarr', name: 'sonarr', host: 'sonarr.example' }),
    ]
    expect(checksOf('arr', ['radarr', 'sonarr'], all).map((c) => c.key)).toEqual(['home-media_radarr', 'x_sonarr'])
    expect(checksOf('home-portal', ['home-portal'], all).map((c) => c.key)).toEqual(['public-apps_home-portal'])
    expect(checksOf('estate-dashboard', [], all).map((c) => c.key)).toEqual(['public-apps_estate'])
  })

  it('take one without a host by its name alone', () => {
    const bare = check({ name: 'arr' })
    delete bare.host
    expect(checksOf('arr', [], [bare])).toEqual([bare])
  })

  it("never take another Project's namespace that only starts the same", () => {
    const other = check({ host: 'x.arr-systemx.svc.cluster.local', name: 'x' })
    expect(checksOf('arr', [], [other])).toEqual([])
  })
})

describe('a check', () => {
  it('that answered is up', () => {
    expect(checkHealth(check(), now)).toMatchObject({ tone: 'ok', word: 'Up' })
  })

  it('that failed is down, with why', () => {
    expect(checkHealth(check({ up: false, error: 'context deadline exceeded' }), now)).toMatchObject({
      tone: 'crit',
      word: 'Down',
      detail: 'context deadline exceeded',
    })
  })

  it('that failed without saying why is down', () => {
    expect(checkHealth(check({ up: false }), now)).toEqual({ tone: 'crit', word: 'Down' })
  })

  it('that answered but failed one in its recent probes is flapping', () => {
    expect(checkHealth(check({ recent: [true, false, true] }), now)).toMatchObject({ tone: 'warn', word: 'Flapping' })
  })

  it('that has not probed for ten minutes is stale', () => {
    expect(checkHealth(check({ at: '2026-10-19T20:45:00Z' }), now)).toMatchObject({ tone: 'warn', word: 'Stale' })
  })

  it('that has not probed yet is waiting', () => {
    const fresh = check({ up: false, recent: [] })
    delete fresh.at
    expect(checkHealth(fresh, now)).toMatchObject({ tone: 'idle', word: 'Not probed yet' })
  })
})
