import { describe, expect, it } from 'vitest'
import type { Pod, PodContainer, Update } from '../../../infrastructure/api/types.gen'
import { runningNow } from './running'

const now = Date.parse('2026-10-20T12:00:00Z')

const container = (over: Partial<PodContainer> = {}): PodContainer => ({
  name: 'sonarr',
  image: 'ghcr.io/jorisjonkers-dev/sonarr@sha256:1',
  init: false,
  sidecar: false,
  ready: true,
  state: 'running',
  restarts: 0,
  probes: { readiness: true, liveness: false, startup: false },
  versionSource: 'lock',
  version: '4.0.19',
  ...over,
})

const pod = (over: Partial<Pod> = {}): Pod => ({
  name: 'sonarr-566dd79995-87ln4',
  process: 'sonarr',
  version: '1.0.4',
  owner: 'ReplicaSet',
  phase: 'Running',
  ready: true,
  deleting: false,
  containers: [
    container(),
    container({
      name: 'backup',
      image: 'ghcr.io/jorisjonkers-dev/delivery/file-backup@sha256:2',
      sidecar: true,
      version: '0.9.2',
    }),
  ],
  ...over,
})

describe('what runs now', () => {
  it('is each Process of the release pinned, with its own container and its sidecars under it', () => {
    const { current, older } = runningNow([pod(), pod({ name: 'sonarr-2', ready: false })], '1.0.4', [], now)
    expect(older).toEqual([])
    expect(current).toHaveLength(1)
    const sonarr = current[0]
    expect(sonarr).toMatchObject({ process: 'sonarr', ready: 1, total: 2, tone: 'warn' })
    expect(sonarr?.main).toMatchObject({
      container: 'sonarr',
      repository: 'ghcr.io/jorisjonkers-dev/sonarr',
      version: '4.0.19',
      ready: 2,
      total: 2,
    })
    expect(sonarr?.sidecars).toEqual([
      {
        container: 'backup',
        repository: 'ghcr.io/jorisjonkers-dev/delivery/file-backup',
        version: '0.9.2',
        ready: 2,
        total: 2,
      },
    ])
  })

  it('names a newer version Renovate has ready for a container', () => {
    const update: Update = {
      dependency: 'linuxserver/sonarr',
      version: '4.0.20',
      repository: 'JorisJonkers-dev/arr',
      number: 27,
      url: 'https://github.com/JorisJonkers-dev/arr/pull/27',
      title: 'update',
      opened: '2026-10-20T07:00:00Z',
    }
    expect(runningNow([pod()], '1.0.4', [update], now).current[0]?.main.newer).toBe('4.0.20')
  })

  it('sets the pods of an older release apart, newest first, and says when they stop', () => {
    const { current, older } = runningNow(
      [
        pod(),
        pod({ name: 'sonarr-old', version: '1.0.3', deleting: true }),
        pod({ name: 'sonarr-older', version: '1.0.2' }),
        pod({ name: 'nameless', version: undefined }),
      ],
      '1.0.4',
      [],
      now,
    )
    expect(current[0]?.total).toBe(2)
    expect(older.map((o) => [o.release, o.stopping])).toEqual([
      ['1.0.3', true],
      ['1.0.2', false],
    ])
  })

  it('takes every pod for the release where none is pinned, and leaves Jobs out', () => {
    const { current } = runningNow(
      [pod({ version: '9.9.9' }), pod({ name: 'migrate', owner: 'Job', process: 'migration' })],
      undefined,
      [],
      now,
    )
    expect(current.map((g) => g.process)).toEqual(['sonarr'])
  })

  it('folds Flagger’s primary into its Process, by name where the pod has no Process label, sorted', () => {
    const { current } = runningNow(
      [pod({ name: 'b-primary-1', process: 'b-primary' }), pod({ name: 'a-0', process: '' })],
      '1.0.4',
      [],
      now,
    )
    expect(current.map((g) => g.process)).toEqual(['a-0', 'b'])
  })

  it('names a Process whose pods hold only sidecars by the Process, with no version', () => {
    const { current } = runningNow(
      [pod({ containers: [container({ name: 'backup', sidecar: true })] })],
      '1.0.4',
      [],
      now,
    )
    expect(current[0]?.main).toEqual({ container: 'sonarr', repository: 'sonarr', ready: 0, total: 0 })
  })
})
