import { describe, expect, it } from 'vitest'
import type { Pod, Release, RolloutWindow, Run, Stage } from '../../../infrastructure/api/types.gen'
import type { ReleaseView } from './releases'
import {
  clock,
  containerAt,
  containerTrack,
  gateConditions,
  ifItFails,
  laneAt,
  lanesOf,
  lastMoment,
  orderAt,
  rolloutState,
  runAt,
  runsOf,
  runTrack,
  slots,
  windowOf,
  type Lane,
  type LaneContainer,
  type RunGroup,
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
  recovered: false,
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
      containers: [],
    },
    {
      pod: 'sonarr-new',
      process: 'sonarr',
      start: at(55),
      ready: at(140),
      checks: checks(55, 200, 140),
      containers: [
        {
          name: 'backup',
          kind: 'sidecar',
          changes: [
            { at: at(55), state: 'running' },
            { at: at(60), state: 'ready' },
          ],
        },
      ],
    },
  ],
  weights: [],
  runs: [],
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
      containers: [
        {
          name: 'backup',
          kind: 'sidecar',
          oldVersion: '0.9.1',
          newVersion: '0.9.2',
          changes: [
            { at: 55, state: 'running' },
            { at: 60, state: 'ready' },
          ],
        },
      ],
    })
  })

  it('names the releases where no pod says which container versions ran', () => {
    expect(lane({ pods: [], window: { ...continuous, pods: continuous.pods.slice(0, 1) } })).toMatchObject({
      oldVersion: '1.0.3',
      newVersion: '1.0.4',
      containers: [],
    })
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
      containers: [
        { name: 'backup', kind: 'sidecar', oldVersion: '0.9.1' },
        { name: 'exporter', kind: 'sidecar', newVersion: '0.1.0', changes: [] },
      ],
    })
  })

  it('has no stop or end while an old pod still runs, and nothing new before a new pod starts', () => {
    const l = lane({
      window: {
        pods: [{ pod: 'sonarr-old', process: 'sonarr', start: at(-60), checks: [], containers: [] }],
        weights: [],
        runs: [],
      },
    })
    expect(l.start).toBeUndefined()
    expect(l.ready).toBeUndefined()
    expect(l.stop).toBeUndefined()
    expect(l.gone).toBeUndefined()
  })

  it('fails with its release when its new pod never turned ready', () => {
    const window: RolloutWindow = {
      pods: [{ pod: 'sonarr-new', process: 'sonarr', start: at(50), checks: checks(50, 100), containers: [] }],
      weights: [],
      runs: [],
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
      containers: [],
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
    expect(laneAt({ process: 'x', weights: [], checks: [], containers: [] }, 500).new.word).toBe('Waiting')
    const l: Lane = { process: 'x', start: 0, ready: 10, stop: 20, weights: [], checks: [], containers: [] }
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
      containers: [],
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
      containers: [],
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

describe('a lane’s containers', () => {
  const initPod: Pod = {
    ...pod('1.0.4', '4.0.19', '0.9.2'),
    containers: [
      container('sonarr', '4.0.19'),
      container('backup', '0.9.2', true),
      { ...container('vault-agent-init', '1.18.0'), init: true },
      { ...container('vpn', '3.41.3'), init: true },
    ],
  }
  const window: RolloutWindow = {
    pods: [
      {
        pod: 'sonarr-new',
        process: 'sonarr',
        start: at(55),
        checks: [],
        containers: [
          { name: 'vpn', kind: 'sidecar', changes: [{ at: at(55), state: 'ready' }] },
          {
            name: 'vault-agent-init',
            kind: 'init',
            changes: [
              { at: at(50), state: 'waiting' },
              { at: at(52), state: 'running' },
              { at: at(55), state: 'succeeded' },
            ],
          },
        ],
      },
    ],
    weights: [],
    runs: [],
  }

  it('are its sidecars then its init containers, a sidecar the kubelet starts first still a sidecar', () => {
    const l = lane({ window, pods: [initPod] })
    expect(l.containers.map((c) => `${c.name} ${c.kind}`)).toEqual([
      'backup sidecar',
      'vpn sidecar',
      'vault-agent-init init',
    ])
    expect(l.containers[2]).toMatchObject({ newVersion: '1.18.0', changes: [{ at: 50 }, { at: 52 }, { at: 55 }] })
  })

  const sidecar: LaneContainer = {
    name: 'backup',
    kind: 'sidecar',
    changes: [
      { at: 10, state: 'waiting' },
      { at: 20, state: 'running' },
      { at: 30, state: 'ready' },
      { at: 90, state: 'failed' },
      { at: 95, state: 'succeeded' },
    ],
  }
  const init: LaneContainer = {
    name: 'migrate',
    kind: 'init',
    changes: [
      { at: 10, state: 'waiting' },
      { at: 20, state: 'running' },
      { at: 25, state: 'ready' },
      { at: 30, state: 'succeeded' },
    ],
  }

  it('say what a sidecar is doing: waiting, starting, ready, or down', () => {
    expect([5, 10, 20, 30, 90, 95].map((t) => containerAt(sidecar, t))).toEqual([
      { word: 'Not started', tone: 'gone' },
      { word: 'Waiting', tone: 'gone' },
      { word: 'Starting, not ready', tone: 'info' },
      { word: 'Ready', tone: 'ok' },
      { word: 'Failed', tone: 'crit' },
      { word: 'Stopped', tone: 'gone' },
    ])
    expect(containerAt({ ...sidecar, changes: [] }, 50)).toEqual({ word: 'No data', tone: 'gone' })
  })

  it('say an init container runs once and is done, or failed', () => {
    expect([20, 25, 30].map((t) => containerAt(init, t).word)).toEqual(['Running', 'Running', 'Done'])
    expect(containerAt({ ...init, changes: [{ at: 0, state: 'failed' }] }, 5).word).toBe('Failed')
  })

  it('draw a sidecar as each state it held, the last open, and as down once it ended', () => {
    expect(containerTrack(sidecar, 40)).toEqual({
      segments: [
        { from: 10, to: 20, state: 'waiting', open: false },
        { from: 20, to: 30, state: 'running', open: false },
        { from: 30, to: 40, state: 'ready', open: true },
      ],
      marks: [],
    })
    expect(containerTrack(sidecar, 100).segments.slice(-2)).toEqual([
      { from: 90, to: 95, state: 'down', open: false },
      { from: 95, to: 100, state: 'down', open: true },
    ])
  })

  it('draw an init container at work, then where it ended, and nothing after: it is gone', () => {
    expect(containerTrack(init, 100)).toEqual({
      segments: [
        { from: 10, to: 20, state: 'waiting', open: false },
        { from: 20, to: 25, state: 'running', open: false },
        { from: 25, to: 30, state: 'running', open: false },
      ],
      marks: [{ at: 30, outcome: 'succeeded' }],
    })
    expect(containerTrack({ ...init, changes: [{ at: 0, state: 'failed' }] }, 5).marks).toEqual([
      { at: 0, outcome: 'failed' },
    ])
  })
})

describe('a release’s runs', () => {
  const run = (over: Partial<Run>): Run => ({
    pod: 'p',
    job: 'vault-policy-29877720',
    cronJob: 'vault-policy',
    start: at(0),
    end: at(5),
    outcome: 'succeeded',
    ...over,
  })
  const window: RolloutWindow = {
    pods: [],
    weights: [],
    runs: [
      run({ pod: 'b', job: 'vault-policy-29877780', start: at(3600), end: at(3606) }),
      run({ pod: 'a', start: at(40), end: at(44) }),
      run({ pod: 'm', job: 'auth-migration-up', cronJob: undefined, start: at(20), end: at(80), outcome: 'failed' }),
      run({
        pod: 'n',
        job: 'auth-migration-up',
        cronJob: undefined,
        start: at(90),
        end: undefined,
        outcome: 'running',
      }),
    ],
  }

  it('are each under the CronJob that made its Job, else the Job, first run first', () => {
    const groups = runsOf(window, t0)
    expect(groups.map((g) => `${g.kind} ${g.name} ${g.runs.map((r) => r.pod).join('')}`)).toEqual([
      'Job auth-migration-up mn',
      'CronJob vault-policy ab',
    ])
    expect(groups[0]?.runs[1]).toEqual({ pod: 'n', job: 'auth-migration-up', start: 90, outcome: 'running' })
    expect(groups[1]?.runs[0]).toEqual({
      pod: 'a',
      job: 'vault-policy-29877720',
      start: 40,
      end: 44,
      outcome: 'succeeded',
    })
  })

  const migration = runsOf(window, t0)[0] as RunGroup

  it('draw each run as a span while it works and a mark where it ended, nothing before it starts', () => {
    expect(runTrack(migration, 10)).toEqual({ segments: [], marks: [] })
    expect(runTrack(migration, 50)).toEqual({
      segments: [{ from: 20, to: 50, state: 'running', open: true }],
      marks: [],
    })
    expect(runTrack(migration, 100)).toEqual({
      segments: [
        { from: 20, to: 80, state: 'running', open: false },
        { from: 90, to: 100, state: 'running', open: true },
      ],
      marks: [{ at: 80, outcome: 'failed' }],
    })
  })

  it('mark a run whose end nobody saw as unknown', () => {
    const lost: RunGroup = {
      name: 'x',
      kind: 'Job',
      runs: [{ pod: 'x', job: 'x', start: 0, end: 5, outcome: 'unknown' }],
    }
    expect(runTrack(lost, 10).marks).toEqual([{ at: 5, outcome: 'unknown' }])
    expect(
      runTrack({ ...lost, runs: [{ ...lost.runs[0], outcome: 'running' } as RunGroup['runs'][number]] }, 10).marks,
    ).toEqual([{ at: 5, outcome: 'unknown' }])
  })

  it('say where the latest run stands, and what it did', () => {
    expect(runAt(migration, 10)).toEqual({ word: 'Not run yet', tone: 'gone', detail: 'first run at 0:20' })
    expect(runAt(migration, 50)).toEqual({ word: 'Running', tone: 'info', detail: '1 run · running since 0:20' })
    expect(runAt(migration, 85)).toEqual({ word: 'Failed', tone: 'crit', detail: '1 run · last 0:20–1:20, 1:00' })
    expect(runAt(migration, 95).detail).toBe('2 runs · running since 1:30')
    const policy = runsOf(window, t0)[1] as RunGroup
    expect(runAt(policy, 50)).toEqual({ word: 'Succeeded', tone: 'ok', detail: '1 run · last 0:40–0:44, 4s' })
    const gone: RunGroup = {
      name: 'x',
      kind: 'Job',
      runs: [{ pod: 'x', job: 'x', start: 0, end: 5, outcome: 'unknown' }],
    }
    expect(runAt(gone, 10)).toEqual({
      word: 'Gone',
      tone: 'gone',
      detail: '1 run · last 0:00–0:05, 5s, how it ended unknown',
    })
    expect(
      runAt({ ...gone, runs: [{ ...gone.runs[0], outcome: 'running' } as RunGroup['runs'][number]] }, 10).detail,
    ).toContain('how it ended unknown')
    expect(runAt({ name: 'x', kind: 'Job', runs: [] }, 10).detail).toBe('first run at 0:00')
  })

  it('carry a release on to the last run’s end', () => {
    expect(lastMoment([], 150, runsOf(window, t0))).toBe(3606)
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
    expect(rolloutState(view({ state: 'live', recovered: true }))).toEqual({ word: 'Recovered', tone: 'warn' })
  })

  it('counts its time in minutes and seconds', () => {
    expect(clock(75)).toBe('1:15')
    expect(clock(-3)).toBe('0:00')
  })
})
