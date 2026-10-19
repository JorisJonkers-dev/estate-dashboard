import { describe, expect, it } from 'vitest'
import type { Check, Pod, Source, Unit } from '../../../infrastructure/api/types.gen'
import { checkHealth, checksOf, stackHealth } from './health'

const now = Date.parse('2026-10-19T21:00:00Z')
const digest = 'sha256:9671a9c5'

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

const source = (over: Partial<Source> = {}): Source => ({
  name: 'project-arr',
  url: 'oci://ghcr.io/jorisjonkers-dev/render/arr',
  digest,
  revision: digest,
  ready: true,
  ...over,
})

const unit = (over: Partial<Unit> = {}): Unit => ({
  name: 'apps-arr',
  source: 'project-arr',
  path: './apps/arr',
  dependsOn: [],
  applied: digest,
  version: '1.0.4',
  ready: true,
  ...over,
})

const pod = (over: Partial<Pod> = {}): Pod => ({
  name: 'radarr-1',
  process: 'radarr',
  phase: 'Running',
  ready: true,
  deleting: false,
  containers: [
    {
      name: 'radarr',
      image: 'radarr:5',
      init: false,
      ready: true,
      state: 'running',
      restarts: 0,
      probes: { readiness: true, liveness: true, startup: false },
    },
  ],
  ...over,
})

const healthy = { source: source(), units: [unit()], pods: [pod()], checks: [check()], now }

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

describe("a stack's health", () => {
  it('is healthy when delivery, every container and every check are', () => {
    const h = stackHealth(healthy)
    expect(h).toMatchObject({ tone: 'ok', word: 'Healthy' })
    expect(h.parts.map((p) => [p.name, p.tone, p.word])).toEqual([
      ['Delivery', 'ok', 'Applied'],
      ['Containers', 'ok', '1 of 1 ready'],
      ['Online', 'ok', '1 of 1 up'],
    ])
  })

  it('is down when a check fails', () => {
    expect(stackHealth({ ...healthy, checks: [check(), check({ key: 'b', up: false })] })).toMatchObject({
      tone: 'crit',
      word: 'Down',
    })
    expect(stackHealth({ ...healthy, checks: [check({ up: false })] }).parts[2]?.word).toBe('1 of 1 down')
  })

  it('is down when nothing runs', () => {
    const h = stackHealth({ ...healthy, pods: [] })
    expect(h.tone).toBe('crit')
    expect(h.parts[1]).toMatchObject({ tone: 'crit', word: 'Nothing running' })
  })

  it('is degraded when a container is not ready', () => {
    const h = stackHealth({ ...healthy, pods: [pod(), pod({ name: 'radarr-2', ready: false })] })
    expect(h).toMatchObject({ tone: 'warn', word: 'Degraded' })
    expect(h.parts[1]?.word).toBe('1 of 2 ready')
  })

  it("leaves a Job's pods out of the count", () => {
    const run = pod({ name: 'backup-1', owner: 'Job', phase: 'Succeeded', ready: false })
    expect(stackHealth({ ...healthy, pods: [pod(), run] }).parts[1]?.word).toBe('1 of 1 ready')
  })

  it('is down when its source or a unit is not ready, with the reason', () => {
    const failed = stackHealth({
      ...healthy,
      units: [unit({ ready: false, reason: 'HealthCheckFailed', message: 'timeout' })],
    })
    expect(failed.tone).toBe('crit')
    expect(failed.parts[0]).toMatchObject({ word: 'HealthCheckFailed', detail: 'apps-arr: timeout' })
    expect(
      stackHealth({ ...healthy, source: source({ ready: false, reason: 'VerificationError' }) }).parts[0]?.word,
    ).toBe('VerificationError')
  })

  it('says a source or a unit is not ready where Flux gives no reason', () => {
    const quietSource = stackHealth({ ...healthy, source: source({ ready: false, message: 'no signature' }) }).parts[0]
    expect(quietSource).toMatchObject({ word: 'Not ready', detail: 'no signature' })
    const quietUnit = stackHealth({ ...healthy, units: [unit({ ready: false })] }).parts[0]
    expect(quietUnit).toMatchObject({ tone: 'crit', word: 'Not ready' })
    expect(quietUnit).not.toHaveProperty('detail')
  })

  it('is updating while Flux fetches or applies a new pin', () => {
    const fetching = stackHealth({ ...healthy, source: source({ digest: 'sha256:new' }) })
    expect(fetching).toMatchObject({ tone: 'info', word: 'Updating' })
    expect(fetching.parts[0]?.word).toBe('Fetching the pin')
    expect(stackHealth({ ...healthy, units: [unit({ applied: 'sha256:old' })] }).parts[0]?.word).toBe(
      'Applying the pin',
    )
  })

  it('reads a Flux revision that carries a tag before its digest', () => {
    expect(stackHealth({ ...healthy, source: source({ revision: `latest@${digest}` }) }).parts[0]?.word).toBe('Applied')
  })

  it('without a source is not pinned', () => {
    expect(stackHealth({ ...healthy, source: undefined }).parts[0]).toMatchObject({ tone: 'idle', word: 'Not pinned' })
  })

  it('says a part it could not read is not read, rather than down', () => {
    const h = stackHealth({ ...healthy, pods: [], checks: [], unread: { pods: true, checks: true } })
    expect(h.parts.slice(1).map((p) => [p.tone, p.word])).toEqual([
      ['idle', 'Not read'],
      ['idle', 'Not read'],
    ])
    expect(h.tone).toBe('ok')
  })

  it('is unknown when nothing could be read', () => {
    const h = stackHealth({ ...healthy, source: undefined, unread: { pods: true, checks: true } })
    expect(h).toMatchObject({ tone: 'idle', word: 'Unknown' })
  })

  it('without checks says so, and counts them out', () => {
    const h = stackHealth({ ...healthy, checks: [] })
    expect(h.tone).toBe('ok')
    expect(h.parts[2]).toMatchObject({ tone: 'idle', word: 'No checks' })
  })
})
