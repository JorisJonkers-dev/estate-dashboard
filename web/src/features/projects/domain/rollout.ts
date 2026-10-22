import type { ContainerChange, Pod, Release, RolloutWindow, Run, Stage } from '../../../infrastructure/api/types.gen'
import type { StripBlock } from '../../../shared/strip'
import type { Tone } from '../../../shared/tone'
import type { ReleaseView } from './releases'

/** How many health-check slots a lane's strip keeps, so it keeps its width while a release runs. */
export const slots = 40

/** One Process's handover from the old release to the new, in seconds from the release's start. */
export interface Lane {
  process: string
  oldVersion?: string
  newVersion?: string
  /** When the new pod started, turned ready, and the old one stopped serving and was gone. */
  start?: number
  ready?: number
  stop?: number
  gone?: number
  /** When the new pod failed: a failed release whose new pod never turned ready. */
  fail?: number
  /** Each moment Flagger moved the canary's weight, and when it promoted; none where nothing is a canary. */
  weights: { at: number; weight: number }[]
  promote?: number
  /** The new pod's readiness every five seconds. */
  checks: { at: number; ready: boolean }[]
  /** Its sidecars, then its init containers, each with the versions it ran and how it went. */
  containers: LaneContainer[]
}

/** One sidecar or init container of a lane's pod: each change of its state, in seconds from the release's start. */
export interface LaneContainer {
  name: string
  kind: 'sidecar' | 'init'
  oldVersion?: string
  newVersion?: string
  changes: { at: number; state: ContainerChange['state'] }[]
}

const seconds = (at: string, t0: number) => Math.round((Date.parse(at) - t0) / 1000)

/** The version of a Process's own container, and its sidecars' and init containers', in the pods of one release. */
function versionsOf(pods: Pod[], process: string, release: string | undefined) {
  const of = pods.find((p) => p.version === release && (p.process || p.name).replace(/-primary$/, '') === process)
  const main = of?.containers.find((c) => !c.sidecar && !c.init)
  return {
    main: main?.version,
    others: new Map(
      (of?.containers ?? [])
        .filter((c) => c.sidecar || c.init)
        .map((c) => [c.name, { version: c.version, kind: c.init ? ('init' as const) : ('sidecar' as const) }]),
    ),
  }
}

/**
 * A lane's sidecars and init containers: every one either release's pods name, or the watched pod
 * ran, sidecars first. The window says best what kind each is: a sidecar the kubelet starts as an
 * init container is still a sidecar.
 */
function containersOf(
  spans: RolloutWindow['pods'][number]['containers'],
  before: ReturnType<typeof versionsOf>['others'],
  after: ReturnType<typeof versionsOf>['others'],
  t0: number,
): LaneContainer[] {
  const kinds = new Map<string, LaneContainer['kind']>()
  for (const [name, c] of [...before, ...after]) kinds.set(name, c.kind)
  for (const c of spans) kinds.set(c.name, c.kind)
  const containers = [...kinds].map(([name, kind]): LaneContainer => {
    const span = spans.find((c) => c.name === name)
    const oldVersion = before.get(name)?.version
    const newVersion = after.get(name)?.version
    return {
      name,
      kind,
      ...(oldVersion ? { oldVersion } : {}),
      ...(newVersion ? { newVersion } : {}),
      changes: (span?.changes ?? []).map((c) => ({ at: seconds(c.at, t0), state: c.state })),
    }
  })
  return containers.sort((a, b) => (a.kind === b.kind ? a.name.localeCompare(b.name) : a.kind === 'sidecar' ? -1 : 1))
}

/**
 * Each Process's lane over a release's window: the pods that started after the release began are
 * its new ones, the rest its old ones. A failed release whose new pod never turned ready failed when
 * its last stage did.
 */
export function lanesOf(input: {
  view: ReleaseView
  from?: string | undefined
  window: RolloutWindow
  pods: Pod[]
  t0: number
  failedAt?: number | undefined
}): Lane[] {
  const { view, window, pods, t0 } = input
  const processes = [...new Set(window.pods.map((p) => p.process))].sort()
  return processes.map((process) => {
    const spans = window.pods.filter((p) => p.process === process)
    const fresh = spans.filter((p) => p.start !== undefined && Date.parse(p.start) >= t0)
    const old = spans.filter((p) => !fresh.includes(p))
    const first = (times: (string | undefined)[]) => {
      const known = times.filter((t) => t !== undefined).map((t) => seconds(t, t0))
      return known.length > 0 ? Math.min(...known) : undefined
    }
    const last = (times: (string | undefined)[]) => {
      const known = times.filter((t) => t !== undefined).map((t) => seconds(t, t0))
      return known.length > 0 && known.length === times.length ? Math.max(...known) : undefined
    }
    const start = first(fresh.map((p) => p.start))
    const ready = first(fresh.map((p) => p.ready))
    // The old pods stop serving when the last of them stops being ready, and are gone when the last ends.
    const stop = last(old.map((p) => p.checks.findLast((c) => c.ready)?.at))
    const gone = last(old.map((p) => p.end))
    const weights = window.weights
      .filter((w) => w.process === process)
      .map((w) => ({ at: seconds(w.at, t0), weight: w.weight }))
    // A canary's weight steps up each round it passes; it falls back to 0 once Flagger promotes it.
    const canary = weights.some((w) => w.weight > 0)
    const firstRound = weights.find((w) => w.weight > 0)?.at ?? 0
    const promote = canary ? weights.findLast((w) => w.weight === 0 && w.at > firstRound)?.at : undefined
    const newVersions = versionsOf(pods, process, view.version)
    const oldVersions = versionsOf(pods, process, input.from)
    // The new pod's containers once one started, the old one's until then.
    const watched = (fresh.length > 0 ? fresh : old).slice(0, 1).flatMap((p) => p.containers)
    return {
      process,
      ...(oldVersions.main ? { oldVersion: oldVersions.main } : input.from ? { oldVersion: input.from } : {}),
      ...(newVersions.main ? { newVersion: newVersions.main } : { newVersion: view.version }),
      ...(start === undefined ? {} : { start }),
      ...(ready === undefined ? {} : { ready }),
      ...(stop === undefined ? {} : { stop }),
      ...(gone === undefined ? {} : { gone }),
      ...(view.state === 'failed' && ready === undefined && input.failedAt !== undefined
        ? { fail: input.failedAt }
        : {}),
      weights: canary ? weights : [],
      ...(promote === undefined ? {} : { promote }),
      checks: (fresh[0]?.checks ?? []).map((c) => ({ at: seconds(c.at, t0), ready: c.ready })),
      containers: containersOf(watched, oldVersions.others, newVersions.others, t0),
    }
  })
}

/**
 * A stretch of time drawn on a track, in seconds from the release's start: a container or a run
 * waiting, at work, ready, or down; open while it still goes on.
 */
export interface Segment {
  from: number
  to: number
  state: 'waiting' | 'running' | 'ready' | 'down'
  open: boolean
}

/** How one-shot work ended, marked where it did. */
export interface Mark {
  at: number
  outcome: 'succeeded' | 'failed' | 'unknown'
}

/** What a track draws up to a moment: one-shot work as spans that end, never as health checks. */
export interface Track {
  segments: Segment[]
  marks: Mark[]
}

/** What a sidecar or an init container was doing at t, in words. */
export function containerAt(c: LaneContainer, t: number): PodState {
  const state = c.changes.findLast((ch) => ch.at <= t)?.state
  if (state === undefined)
    return c.changes.length > 0 ? { word: 'Not started', tone: 'gone' } : { word: 'No data', tone: 'gone' }
  const init = c.kind === 'init'
  switch (state) {
    case 'waiting':
      return { word: 'Waiting', tone: 'gone' }
    case 'running':
      return init ? { word: 'Running', tone: 'info' } : { word: 'Starting, not ready', tone: 'info' }
    case 'ready':
      return init ? { word: 'Running', tone: 'info' } : { word: 'Ready', tone: 'ok' }
    case 'succeeded':
      return init ? { word: 'Done', tone: 'ok' } : { word: 'Stopped', tone: 'gone' }
    case 'failed':
      return { word: 'Failed', tone: 'crit' }
  }
}

/**
 * A sidecar's or init container's track up to t: each state it held as a segment, and where an
 * init container ended, a mark. An init container that ended draws nothing after: it is gone, not
 * down. A sidecar that ended is down.
 */
export function containerTrack(c: LaneContainer, t: number): Track {
  const segments: Segment[] = []
  const marks: Mark[] = []
  const seen = c.changes.filter((ch) => ch.at <= t)
  seen.forEach((ch, i) => {
    const next = seen[i + 1]?.at
    const to = next ?? t
    const ended = ch.state === 'succeeded' || ch.state === 'failed'
    if (c.kind === 'init' && ended) {
      marks.push({ at: ch.at, outcome: ch.state === 'succeeded' ? 'succeeded' : 'failed' })
      return
    }
    segments.push({ from: ch.at, to, state: segmentOf(ch.state, c.kind), open: next === undefined })
  })
  return { segments, marks }
}

/** What a segment of a container's track is: an init container at work is running, ready or not; a sidecar that ended is down. */
function segmentOf(state: ContainerChange['state'], kind: LaneContainer['kind']): Segment['state'] {
  if (state === 'waiting' || state === 'running') return state
  if (state === 'ready') return kind === 'init' ? 'running' : 'ready'
  return 'down'
}

/** One Job, or the CronJob that made it, and each run of it in the window, in seconds from the release's start. */
export interface RunGroup {
  name: string
  kind: 'CronJob' | 'Job'
  runs: { pod: string; job: string; start: number; end?: number; outcome: Run['outcome'] }[]
}

/** The window's runs, each under the CronJob that made its Job, else its Job: first run first. */
export function runsOf(window: RolloutWindow, t0: number): RunGroup[] {
  const groups = new Map<string, RunGroup>()
  for (const r of [...window.runs].sort((a, b) => Date.parse(a.start) - Date.parse(b.start))) {
    const name = r.cronJob ?? r.job
    const key = `${r.cronJob ? 'CronJob' : 'Job'}/${name}`
    const group = groups.get(key) ?? { name, kind: r.cronJob ? 'CronJob' : 'Job', runs: [] }
    group.runs.push({
      pod: r.pod,
      job: r.job,
      start: seconds(r.start, t0),
      ...(r.end ? { end: seconds(r.end, t0) } : {}),
      outcome: r.outcome,
    })
    groups.set(key, group)
  }
  return [...groups.values()]
}

/** A group's runs up to t: each a span while it works, and a mark where it ended. */
export function runTrack(group: RunGroup, t: number): Track {
  const started = group.runs.filter((r) => r.start <= t)
  const ended = (r: RunGroup['runs'][number]) => (r.end !== undefined && r.end <= t ? r.end : undefined)
  return {
    segments: started.map((r) => {
      const end = ended(r)
      return { from: r.start, to: end ?? t, state: 'running', open: end === undefined }
    }),
    marks: started.flatMap((r) => {
      const at = ended(r)
      if (at === undefined) return []
      return [{ at, outcome: r.outcome === 'succeeded' || r.outcome === 'failed' ? r.outcome : 'unknown' }]
    }),
  }
}

const lasted = (s: number) => (s < 60 ? `${String(Math.max(0, s))}s` : clock(s))

/** Where a group's latest run stands at t, and what it did, in words. */
export function runAt(group: RunGroup, t: number): PodState & { detail: string } {
  const started = group.runs.filter((r) => r.start <= t)
  const last = started.at(-1)
  const count = started.length === 1 ? '1 run' : `${String(started.length)} runs`
  if (!last) return { word: 'Not run yet', tone: 'gone', detail: `first run at ${clock(group.runs[0]?.start ?? 0)}` }
  if (last.end === undefined || last.end > t)
    return { word: 'Running', tone: 'info', detail: `${count} · running since ${clock(last.start)}` }
  const took = `${clock(last.start)}–${clock(last.end)}, ${lasted(last.end - last.start)}`
  const word: PodState =
    last.outcome === 'succeeded'
      ? { word: 'Succeeded', tone: 'ok' }
      : last.outcome === 'failed'
        ? { word: 'Failed', tone: 'crit' }
        : { word: 'Gone', tone: 'gone' }
  return {
    ...word,
    detail: `${count} · last ${took}${last.outcome === 'unknown' || last.outcome === 'running' ? ', how it ended unknown' : ''}`,
  }
}

/** A state a pod of a lane is in, and its colour. */
export interface PodState {
  word: string
  tone: Tone | 'gone'
}

/** Where a lane stands at one moment of its release. */
export interface LaneAt {
  /** The share of traffic on the new pod, in percent. */
  share: number
  old: PodState
  new: PodState
  /** What happens next, in words. */
  next: string
  failed: boolean
  blocks: StripBlock[]
}

const clock = (s: number) =>
  `${String(Math.floor(Math.max(0, s) / 60))}:${String(Math.max(0, s) % 60).padStart(2, '0')}`

/** The lane at t seconds into its release: the model the prototype draws. */
export function laneAt(lane: Lane, t: number): LaneAt {
  const failed = lane.fail !== undefined && t >= lane.fail
  const ready = lane.ready ?? Infinity
  const start = lane.start ?? Infinity
  const canary = lane.weights.length > 0
  const promote = lane.promote ?? Infinity
  const weight = lane.weights.findLast((w) => w.at <= t)?.weight ?? 0
  const rounds = lane.weights.filter((w) => w.at <= t && w.weight > 0).length
  const share = failed || t < ready ? 0 : canary ? (t < promote ? weight : 100) : t < (lane.stop ?? Infinity) ? 50 : 100
  const fresh: PodState = failed
    ? { word: 'Failed', tone: 'crit' }
    : t < start
      ? { word: 'Waiting', tone: 'gone' }
      : t < ready
        ? { word: 'Starting', tone: 'info' }
        : canary && t < promote
          ? { word: share > 0 ? `Canary · ${String(share)}%` : 'Ready, no traffic', tone: 'warn' }
          : { word: 'Serving', tone: 'ok' }
  const old: PodState =
    share < 100
      ? { word: 'Serving', tone: 'ok' }
      : t < (lane.gone ?? Infinity)
        ? { word: 'Draining', tone: 'back' }
        : { word: 'Stopped', tone: 'gone' }
  const next = failed
    ? 'never turned ready · the old pod keeps all traffic'
    : t < start
      ? canary
        ? 'waits its turn'
        : 'waits for the unit'
      : t < ready
        ? 'starting · waits for its readiness check'
        : canary && t < promote
          ? rounds > 0
            ? `canary round ${String(rounds)} · checks passing`
            : 'ready · first canary round next'
          : share < 100
            ? 'both pods serving · the old pod stops next'
            : t < (lane.gone ?? Infinity)
              ? 'all traffic on the new pod · the old pod drains'
              : 'switched'
  const seen = lane.checks.filter((c) => c.at <= t).slice(-slots)
  const blocks: StripBlock[] = seen.map((c) => ({
    tone: lane.fail !== undefined && !c.ready ? 'crit' : c.at < ready ? 'info' : c.ready ? 'ok' : 'warn',
    label: `${clock(c.at)} · ${c.ready ? 'check passed' : c.at < ready ? 'starting, not ready' : 'check failed'}`,
  }))
  while (blocks.length < slots) blocks.push({ tone: 'empty', label: 'no check yet' })
  return { share, old, new: fresh, next, failed, blocks }
}

/** One step of a release's order at one moment. */
export interface OrderStep {
  title: string
  at: number
  state: 'done' | 'running' | 'waiting' | 'failed'
  /** What a barrier checked before the release could go on; none for an ordinary step. */
  conditions?: { label: string; met: boolean }[]
}

const failedStage = (s: Stage) => s.state === 'failure' || s.state === 'error'
const barrier = /gate|waits for flagger/i

/** The conditions the Release Gate checks before a gated release switches. */
export function gateConditions(release: Release | undefined): { label: string; met: boolean }[] {
  if (!release?.migration) return []
  const tested = release.migration.testedAgainst
  return [
    {
      label: `every primary runs ${tested ?? 'the tested release'}`,
      met: tested !== undefined && release.serving === tested,
    },
    { label: 'no non-transactional changeset', met: !release.migration.nonTransactional },
  ]
}

/** The release's stages, oldest first, each done, running, waiting or failed at t; a barrier names what it checked. */
export function orderAt(view: ReleaseView, release: Release | undefined, t0: number, t: number): OrderStep[] {
  const stages = [...view.stages].reverse()
  const conditions = gateConditions(release)
  return stages.map((s, n) => {
    const at = seconds(s.at, t0)
    const reached = t >= at
    const previous = stages[n - 1]
    const state: OrderStep['state'] = reached
      ? failedStage(s)
        ? 'failed'
        : n === stages.length - 1 && view.state === 'rolling'
          ? 'running'
          : 'done'
      : previous === undefined || t >= seconds(previous.at, t0)
        ? 'running'
        : 'waiting'
    return {
      title: s.description,
      at,
      state,
      ...(barrier.test(s.description) && conditions.length > 0 ? { conditions } : {}),
    }
  })
}

/** What a failed release undoes, or that the old pods keep serving. */
export function ifItFails(project: string, from: string | undefined, release: Release | undefined) {
  if (release?.migration)
    return {
      text: 'The gate undoes the migration only when all of these hold:',
      conditions: [
        'no pod of the new release has taken traffic',
        release.migration.nonTransactional
          ? 'no changeset ran outside a transaction (one did: the gate holds it)'
          : 'every changeset ran in a transaction',
        'a backup was taken before the migration',
      ],
    }
  if (release && release.members.length > 0)
    return {
      text: 'A canary that fails its checks sends all traffic back to the old pods; nothing else is undone.',
      conditions: [],
    }
  return {
    text: `${project} switches continuously, so a release that fails stops with the old pods still serving. Nothing is undone${from ? `; roll back to go to ${from}` : ''}.`,
    conditions: [],
  }
}

/** Where a release stands, in the pill on its page. */
export function rolloutState(view: ReleaseView): { word: string; tone: Tone } {
  if (view.state === 'rolling') return { word: 'Switching in', tone: 'info' }
  if (view.state === 'failed') return { word: 'Failed', tone: 'crit' }
  if (view.recovered) return { word: 'Recovered', tone: 'warn' }
  return { word: 'Done', tone: 'ok' }
}

/** A release's window: a minute before its first stage to two after its last, or to now while it runs, two hours at most. */
export function windowOf(view: ReleaseView, now: number): { from: string; to: string; t0: number; end: number } {
  const times = view.stages.map((s) => Date.parse(s.at))
  const t0 = Math.min(Date.parse(view.at), ...times)
  const rolling = view.state === 'rolling'
  const finish = rolling ? now : Math.max(t0, ...times)
  // The read covers two hours at most, and the window opens a minute before the release.
  const to = Math.min(finish + (rolling ? 0 : 120_000), t0 + 2 * 3600_000 - 60_000, now)
  return {
    from: new Date(t0 - 60_000).toISOString(),
    to: new Date(Math.max(to, t0 + 1000)).toISOString(),
    t0,
    end: Math.max(1, Math.round((Math.min(finish, to) - t0) / 1000)),
  }
}

/** How long a release ran: to its last stage, or to the last thing a lane or a run did after it. */
export function lastMoment(lanes: Lane[], end: number, runs: RunGroup[] = []): number {
  return Math.max(
    end,
    ...lanes.flatMap((l) => [l.ready, l.stop, l.gone, l.promote, l.fail].filter((x) => x !== undefined)),
    ...runs.flatMap((g) => g.runs.map((r) => r.end ?? r.start)),
  )
}

export { clock }
