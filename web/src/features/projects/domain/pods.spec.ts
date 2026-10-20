import { describe, expect, it } from 'vitest'
import type { Pod, PodContainer } from '../../../infrastructure/api/types.gen'
import { containerHealth, podHealth, podRole, processes } from './pods'

const now = Date.parse('2026-10-19T21:00:00Z')
const ago = (minutes: number) => new Date(now - minutes * 60_000).toISOString()

const container = (over: Partial<PodContainer> = {}): PodContainer => ({
  name: 'radarr',
  image: 'radarr:5',
  init: false,
  sidecar: false,
  versionSource: 'unknown',
  ready: true,
  state: 'running',
  since: ago(120),
  restarts: 0,
  probes: { readiness: true, liveness: true, startup: false },
  ...over,
})

const pod = (over: Partial<Pod> = {}): Pod => ({
  name: 'radarr-566dd79995-jc5mr',
  process: 'radarr',
  version: '1.0.4',
  owner: 'ReplicaSet',
  node: 'enschede-1',
  phase: 'Running',
  started: ago(120),
  ready: true,
  deleting: false,
  containers: [container()],
  ...over,
})

describe('a container', () => {
  it('is healthy when it runs and is ready', () => {
    expect(containerHealth(container(), now)).toMatchObject({ tone: 'ok', word: 'Healthy' })
  })

  it('that waits in a crash loop is down, with the reason', () => {
    const looping = container({ ready: false, state: 'waiting', reason: 'CrashLoopBackOff', restarts: 9 })
    expect(containerHealth(looping, now)).toMatchObject({ tone: 'crit', word: 'CrashLoopBackOff' })
  })

  it('that cannot pull its image is down', () => {
    const pulling = container({ ready: false, state: 'waiting', reason: 'ImagePullBackOff' })
    expect(containerHealth(pulling, now).tone).toBe('crit')
  })

  it('that waits to be created is starting, not down', () => {
    const creating = container({ ready: false, state: 'waiting', reason: 'ContainerCreating' })
    expect(containerHealth(creating, now)).toMatchObject({ tone: 'info', word: 'ContainerCreating' })
    const quiet = container({ ready: false, state: 'waiting' })
    expect(containerHealth(quiet, now)).toMatchObject({ tone: 'info', word: 'Waiting' })
  })

  it('that ended with an error is down, and one that completed is done', () => {
    expect(containerHealth(container({ ready: false, state: 'terminated', reason: 'Error' }), now).tone).toBe('crit')
    expect(containerHealth(container({ ready: false, state: 'terminated', reason: 'Completed' }), now).tone).toBe(
      'idle',
    )
    expect(containerHealth(container({ ready: false, state: 'terminated' }), now)).toMatchObject({
      tone: 'crit',
      word: 'Stopped',
    })
  })

  it('that restarted without saying why says it ended with an error', () => {
    const bare = container({ restarts: 1, last: { exitCode: 2, at: ago(5) } })
    expect(containerHealth(bare, now).detail).toContain('with an error')
  })

  it('that runs but fails its readiness probe is not ready', () => {
    expect(containerHealth(container({ ready: false }), now)).toMatchObject({ tone: 'warn', word: 'Not ready' })
  })

  it('that restarted in the last hour is a warning, with how it ended', () => {
    const restarted = container({ restarts: 2, last: { reason: 'OOMKilled', exitCode: 137, at: ago(10) } })
    expect(containerHealth(restarted, now)).toMatchObject({ tone: 'warn', word: 'Restarted' })
    expect(containerHealth(restarted, now).detail).toContain('OOMKilled')
  })

  it('that restarted long ago is healthy again', () => {
    const settled = container({ restarts: 2, last: { reason: 'Error', exitCode: 1, at: ago(180) } })
    expect(containerHealth(settled, now).tone).toBe('ok')
  })

  it('that is an init container and completed is done', () => {
    const done = container({ init: true, ready: false, state: 'terminated', reason: 'Completed' })
    expect(containerHealth(done, now)).toMatchObject({ tone: 'idle', word: 'Done' })
  })

  it('that the kubelet has not reported yet is starting', () => {
    const fresh = container({ ready: false })
    delete fresh.state
    expect(containerHealth(fresh, now)).toMatchObject({ tone: 'info', word: 'Starting' })
  })
})

describe('a pod', () => {
  it('is healthy when it is ready and every container is', () => {
    expect(podHealth(pod(), now)).toMatchObject({ tone: 'ok', word: 'Healthy' })
  })

  it('takes the worst of its containers', () => {
    const looping = pod({
      ready: false,
      containers: [container(), container({ name: 'vpn', ready: false, state: 'waiting', reason: 'CrashLoopBackOff' })],
    })
    expect(podHealth(looping, now)).toMatchObject({ tone: 'crit', word: 'CrashLoopBackOff' })
  })

  it('that is not ready while its containers are is not ready', () => {
    expect(podHealth(pod({ ready: false }), now)).toMatchObject({ tone: 'warn', word: 'Not ready' })
  })

  it('that is shutting down is stopping', () => {
    expect(podHealth(pod({ deleting: true }), now)).toMatchObject({ tone: 'idle', word: 'Stopping' })
  })

  it('that is not scheduled yet is pending', () => {
    expect(podHealth(pod({ phase: 'Pending', ready: false, containers: [] }), now)).toMatchObject({
      tone: 'info',
      word: 'Pending',
    })
  })

  it('that is pending with a container already reported takes that container', () => {
    const pulling = pod({
      phase: 'Pending',
      ready: false,
      containers: [container({ ready: false, state: 'waiting', reason: 'ImagePullBackOff' })],
    })
    expect(podHealth(pulling, now)).toMatchObject({ tone: 'crit', word: 'ImagePullBackOff' })
    const fresh = container({ ready: false })
    delete fresh.state
    expect(podHealth(pod({ phase: 'Pending', ready: false, containers: [fresh] }), now).word).toBe('Pending')
  })

  it("that is a Job's and succeeded is finished", () => {
    const run = pod({
      owner: 'Job',
      phase: 'Succeeded',
      ready: false,
      containers: [container({ ready: false, state: 'terminated', reason: 'Completed' })],
    })
    expect(podHealth(run, now)).toMatchObject({ tone: 'idle', word: 'Finished' })
  })

  it('that failed is down', () => {
    expect(podHealth(pod({ phase: 'Failed', ready: false }), now)).toMatchObject({ tone: 'crit', word: 'Failed' })
  })
})

describe("a Project's processes", () => {
  it('group its pods by Process, with how many are ready, and its Jobs apart', () => {
    const pods = [
      pod({ name: 'sonarr-1', process: 'sonarr' }),
      pod({ name: 'radarr-1' }),
      pod({ name: 'radarr-2', ready: false }),
      pod({ name: 'backup-29', process: 'backup', owner: 'Job', phase: 'Succeeded', ready: false }),
    ]
    expect(processes(pods).map((p) => [p.process, p.job, p.ready, p.pods.length])).toEqual([
      ['radarr', false, 1, 2],
      ['sonarr', false, 1, 1],
      ['backup', true, 0, 1],
    ])
  })

  it("put Flagger's primary under the Process it switches", () => {
    const pods = [
      pod({ name: 'web-primary-7c9d8-abcde', process: 'web-primary' }),
      pod({ name: 'web-5b6c7-fghij', process: 'web' }),
    ]
    expect(processes(pods).map((p) => [p.process, p.pods.length])).toEqual([['web', 2]])
  })

  it('name a pod without a Process label by its own name', () => {
    expect(processes([pod({ name: 'debug', process: '' })])[0]?.process).toBe('debug')
  })
})

describe('a pod in a list', () => {
  it("is named by Flagger's role, or by its suffix", () => {
    const flagger = [pod({ name: 'web-primary-7c9d8-abcde' }), pod({ name: 'web-5b6c7-fghij' })]
    expect(flagger.map((p) => podRole(p, flagger))).toEqual(['primary', 'canary'])
    expect(podRole(pod(), [pod()])).toBe('jc5mr')
  })
})
