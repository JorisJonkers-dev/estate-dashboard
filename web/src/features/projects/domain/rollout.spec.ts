import { describe, expect, it } from 'vitest'
import type { Pod, Release, RolloutWindow, Stage } from '../../../infrastructure/api/types.gen'
import type { ReleaseView } from './releases'
import {
  clock,
  gateConditions,
  ifItFails,
  laneAt,
  lanesOf,
  lastMoment,
  orderAt,
  rolloutState,
  slots,
  windowOf,
  type Lane,
} from './rollout'

const t0 = Date.parse('2026-10-20T09:00:00Z')
const at = (s: number) => new Date(t0 + s * 1000).toISOString()
const checks = (from: number, to: number, readyAt?: number) =>
  Array.from({ length: Math.floor((to - from) / 5) + 1 }, (_, i) => ({
    at: at(from + i * 5),
    ready: readyAt !== undefined && from + i * 5 >= readyAt,
  }))

const stage = (state: string, description: string, s: number): Stage => ({ state, description, at: at(s) })
const view = (over: Partial<ReleaseView> = {}): ReleaseView => ({
  key: 1,
  version: '1.0.4',
  at: at(0),
  state: 'past',
  pins: 1,
  commit: 'abc',
  stages: [stage('success', 'every pod ready', 150), stage('in_progress', 'arr 1.0.4 pinned', 0)],
  ...over,
})

const container = (name: string, version: string, sidecar = false) => ({
  name,
  image: `ghcr.io/o/${name}@sha256:1`,
  init: false,
  sidecar,
  ready: true,
  restarts: 0,
  probes: { readiness: true, liveness: false, startup: false },
  versionSource: 'lock' as const,
  version,
})
const pod = (version: string, main: string, backup: string): Pod => ({
  name: `sonarr-${version}`,
  process: 'sonarr',
  version,
  ready: true,
  deleting: false,
  containers: [container('sonarr', main), container('backup', backup, true)],
})

const continuous: RolloutWindow = {
  pods: [
    {
      pod: 'sonarr-old',
      process: 'sonarr',
      start: at(-60),
      ready: at(-60),
      end: at(170),
      checks: checks(-60, 170, -60).map((c, i, all) => ({ ...c, ready: i < all.length - 4 })),
    },
    { pod: 'sonarr-new', process: 'sonarr', start: at(55), ready: at(140), checks: checks(55, 200, 140) },
  ],
  weights: [],
}

const lane = (over: Partial<Parameters<typeof lanesOf>[0]> = {}) =>
  lanesOf({
    view: view(),
    from: '1.0.3',
    window: continuous,
    pods: [pod('1.0.4', '4.0.19', '0.9.2'), pod('1.0.3', '4.0.18', '0.9.1')],
    t0,
    ...over,
  })[0] as Lane

describe('a lane', () => {
  it('is a Process handing over from its old pods to the new ones that started after the release began', () => {
    expect(lane()).toMatchObject({
      process: 'sonarr',
      oldVersion: '4.0.18',
      newVersion: '4.0.19',
      start: 55,
      ready: 140,
      stop: 150,
      gone: 170,
      weights: [],
      sidecars: [{ name: 'backup', oldVersion: '0.9.1', newVersion: '0.9.2' }],
    })
  })

  it('names the releases where no pod says which container versions ran', () => {
    expect(lane({ pods: [] })).toMatchObject({ oldVersion: '1.0.3', newVersion: '1.0.4', sidecars: [] })
    expect(lane({ pods: [], from: undefined }).oldVersion).toBeUndefined()
  })

  it('finds a pod without a Process label by its name, and a sidecar only one release ran', () => {
    const fresh: Pod = {
      ...pod('1.0.4', '4.0.19', '0.9.2'),
      process: '',
      name: 'sonarr',
      containers: [container('sonarr', '4.0.19'), container('exporter', '0.1.0', true)],
    }
    expect(lane({ pods: [fresh, pod('1.0.3', '4.0.18', '0.9.1')] })).toMatchObject({
      newVersion: '4.0.19',
      sidecars: [
        { name: 'backup', oldVersion: '0.9.1' },
        { name: 'exporter', newVersion: '0.1.0' },
      ],
    })
  })

  it('has no stop or end while an old pod still runs, and nothing new before a new pod starts', () => {
    const l = lane({
      window: { pods: [{ pod: 'sonarr-old', process: 'sonarr', start: at(-60), checks: [] }], weights: [] },
    })
    expect(l.start).toBeUndefined()
    expect(l.ready).toBeUndefined()
    expect(l.stop).toBeUndefined()
    expect(l.gone).toBeUndefined()
  })

  it('fails with its release when its new pod never turned ready', () => {
    const window: RolloutWindow = {
      pods: [{ pod: 'sonarr-new', process: 'sonarr', start: at(50), checks: checks(50, 100) }],
      weights: [],
    }
    expect(lane({ view: view({ state: 'failed' }), window, failedAt: 650 }).fail).toBe(650)
    expect(lane({ view: view({ state: 'failed' }), window }).fail).toBeUndefined()
  })

  it('carries a canary’s weights, and when Flagger promoted it', () => {
    const window: RolloutWindow = {
      ...continuous,
      weights: [
        { process: 'sonarr', at: at(0), weight: 0 },
        { process: 'sonarr', at: at(100), weight: 20 },
        { process: 'sonarr', at: at(115), weight: 40 },
        { process: 'sonarr', at: at(160), weight: 0 },
        { process: 'other', at: at(100), weight: 20 },
      ],
    }
    const l = lane({ window })
    expect(l.weights.map((w) => w.weight)).toEqual([0, 20, 40, 0])
    expect(l.promote).toBe(160)
  })

  it('is no canary where Flagger never moved its weight', () => {
    expect(lane({ window: { ...continuous, weights: [{ process: 'sonarr', at: at(0), weight: 0 }] } }).weights).toEqual(
      [],
    )
  })

  it('at each moment has its share, its pods’ states and what happens next, for a continuous switch', () => {
    const l = lane()
    const at = (t: number) => laneAt(l, t)
    expect(at(10)).toMatchObject({
      share: 0,
      new: { word: 'Waiting' },
      old: { word: 'Serving' },
      next: 'waits for the unit',
    })
    expect(at(60)).toMatchObject({
      share: 0,
      new: { word: 'Starting' },
      next: 'starting · waits for its readiness check',
    })
    expect(at(145)).toMatchObject({
      share: 50,
      new: { word: 'Serving' },
      next: 'both pods serving · the old pod stops next',
    })
    expect(at(160)).toMatchObject({
      share: 100,
      old: { word: 'Draining' },
      next: 'all traffic on the new pod · the old pod drains',
    })
    expect(at(180)).toMatchObject({ share: 100, old: { word: 'Stopped', tone: 'gone' }, next: 'switched' })
  })

  it('at each moment of a canary moves its share with Flagger’s weight', () => {
    const l: Lane = {
      process: 'auth-api',
      start: 65,
      ready: 90,
      weights: [
        { at: 90, weight: 0 },
        { at: 100, weight: 20 },
        { at: 115, weight: 40 },
        { at: 160, weight: 0 },
      ],
      promote: 160,
      gone: 175,
      checks: [],
      sidecars: [],
    }
    expect(laneAt(l, 10).next).toBe('waits its turn')
    expect(laneAt(l, 95)).toMatchObject({
      share: 0,
      new: { word: 'Ready, no traffic', tone: 'warn' },
      next: 'ready · first canary round next',
    })
    expect(laneAt(l, 120)).toMatchObject({
      share: 40,
      new: { word: 'Canary · 40%' },
      next: 'canary round 2 · checks passing',
    })
    expect(laneAt(l, 165)).toMatchObject({ share: 100, new: { word: 'Serving' }, old: { word: 'Draining' } })
  })

  it('waits while no new pod has started, and drains for as long as an old pod is not seen to end', () => {
    expect(laneAt({ process: 'x', weights: [], checks: [], sidecars: [] }, 500).new.word).toBe('Waiting')
    const l: Lane = { process: 'x', start: 0, ready: 10, stop: 20, weights: [], checks: [], sidecars: [] }
    expect(laneAt(l, 500)).toMatchObject({
      share: 100,
      old: { word: 'Draining' },
      next: 'all traffic on the new pod · the old pod drains',
    })
  })

  it('at a failure keeps all traffic on the old pod', () => {
    const l: Lane = {
      process: 'radarr',
      start: 50,
      fail: 650,
      weights: [],
      checks: [{ at: 50, ready: false }],
      sidecars: [],
    }
    expect(laneAt(l, 700)).toMatchObject({
      share: 0,
      failed: true,
      new: { word: 'Failed', tone: 'crit' },
      old: { word: 'Serving' },
      next: 'never turned ready · the old pod keeps all traffic',
    })
    expect(laneAt(l, 700).blocks[0]).toEqual({ tone: 'crit', label: '0:50 · starting, not ready' })
  })

  it('fills its strip with the new pod’s checks so far, the rest of its slots empty', () => {
    const l: Lane = {
      process: 'x',
      start: 0,
      ready: 10,
      weights: [],
      checks: [
        { at: 0, ready: false },
        { at: 10, ready: true },
        { at: 15, ready: false },
        { at: 300, ready: true },
      ],
      sidecars: [],
    }
    const blocks = laneAt(l, 20).blocks
    expect(blocks).toHaveLength(slots)
    expect(blocks.slice(0, 4)).toEqual([
      { tone: 'info', label: '0:00 · starting, not ready' },
      { tone: 'ok', label: '0:10 · check passed' },
      { tone: 'warn', label: '0:15 · check failed' },
      { tone: 'empty', label: 'no check yet' },
    ])
  })
})

describe('a release’s order', () => {
  const gate: Release = {
    namespace: 'auth-system',
    application: 'auth',
    members: [{ process: 'auth-api', iterations: 2 }],
    migration: { identity: 'auth-migration', testedAgainst: '3.4.2', nonTransactional: false },
    serving: '3.4.2',
  }
  const rolling = view({
    state: 'rolling',
    stages: [
      stage('in_progress', 'both members in analysis', 120),
      stage('in_progress', 'applied by Flux; auth waits for Flagger', 60),
      stage('in_progress', 'auth 3.5.0 pinned', 0),
    ],
  })

  it('is each stage, oldest first, done, running or waiting at a moment, a barrier naming what the gate checked', () => {
    expect(orderAt(rolling, gate, t0, 70)).toEqual([
      { title: 'auth 3.5.0 pinned', at: 0, state: 'done' },
      {
        title: 'applied by Flux; auth waits for Flagger',
        at: 60,
        state: 'done',
        conditions: [
          { label: 'every primary runs 3.4.2', met: true },
          { label: 'no non-transactional changeset', met: true },
        ],
      },
      { title: 'both members in analysis', at: 120, state: 'running' },
    ])
    expect(orderAt(rolling, gate, t0, 0).map((s) => s.state)).toEqual(['done', 'running', 'waiting'])
    expect(orderAt(rolling, gate, t0, 130).at(-1)?.state).toBe('running')
  })

  it('marks a stage that failed, and draws no barrier without a migration', () => {
    const failed = view({
      stages: [
        stage('failure', 'radarr did not become ready, waits for Flagger', 650),
        stage('in_progress', 'pinned', 0),
      ],
    })
    expect(orderAt(failed, undefined, t0, 700)).toEqual([
      { title: 'pinned', at: 0, state: 'done' },
      { title: 'radarr did not become ready, waits for Flagger', at: 650, state: 'failed' },
    ])
  })

  it('checks a migration tested against nothing yet, or one outside a transaction', () => {
    expect(gateConditions({ ...gate, migration: { identity: 'm', nonTransactional: true } })).toEqual([
      { label: 'every primary runs the tested release', met: false },
      { label: 'no non-transactional changeset', met: false },
    ])
    expect(gateConditions(undefined)).toEqual([])
  })

  it('says what a failure undoes: the migration under conditions, a canary, or nothing', () => {
    expect(ifItFails('auth', '3.4.2', gate)).toMatchObject({
      text: 'The gate undoes the migration only when all of these hold:',
    })
    expect(
      ifItFails('auth', '3.4.2', { ...gate, migration: { identity: 'm', nonTransactional: true } }).conditions[1],
    ).toContain('the gate holds it')
    expect(ifItFails('auth', '3.4.2', { ...gate, migration: undefined })).toEqual({
      text: 'A canary that fails its checks sends all traffic back to the old pods; nothing else is undone.',
      conditions: [],
    })
    expect(ifItFails('arr', '1.0.3', undefined).text).toBe(
      'arr switches continuously, so a release that fails stops with the old pods still serving. Nothing is undone; roll back to go to 1.0.3.',
    )
    expect(ifItFails('arr', undefined, { ...gate, members: [], migration: undefined }).text).toMatch(
      /Nothing is undone\.$/,
    )
  })
})

describe('a release’s window', () => {
  it('runs from a minute before its first stage to two minutes after its last', () => {
    expect(windowOf(view(), t0 + 3600_000)).toEqual({ from: at(-60), to: at(270), t0, end: 150 })
  })

  it('follows a release in flight up to now, at most two hours', () => {
    expect(windowOf(view({ state: 'rolling' }), t0 + 200_000)).toMatchObject({ to: at(200), end: 200 })
    expect(windowOf(view({ state: 'rolling' }), t0 + 3 * 3600_000).to).toBe(at(7140))
  })

  it('lasts a second at least', () => {
    expect(windowOf(view({ stages: [] }), t0)).toMatchObject({ to: at(1), end: 1 })
  })

  it('ends at the last thing a lane did, where that came after the last stage', () => {
    expect(lastMoment([lane()], 150)).toBe(170)
    expect(lastMoment([], 150)).toBe(150)
  })
})

describe('a rollout', () => {
  it('reads as switching in, failed or done', () => {
    expect(rolloutState(view({ state: 'rolling' }))).toEqual({ word: 'Switching in', tone: 'info' })
    expect(rolloutState(view({ state: 'failed' }))).toEqual({ word: 'Failed', tone: 'crit' })
    expect(rolloutState(view({ state: 'live' }))).toEqual({ word: 'Done', tone: 'ok' })
  })

  it('counts its time in minutes and seconds', () => {
    expect(clock(75)).toBe('1:15')
    expect(clock(-3)).toBe('0:00')
  })
})
