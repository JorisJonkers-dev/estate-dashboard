import type { Release, Rollout, Stage, Unit } from '../../../infrastructure/api/types.gen'
import type { HeldBack } from './project'

/** Where a release stands now, read from its stages and the unit. */
export type ReleaseState = 'live' | 'rolling' | 'failed' | 'past'

/** One release of a Project. Rollouts of the same version are folded in. */
export interface ReleaseView {
  key: number
  version: string
  at: string
  state: ReleaseState
  /** A stage failed, and a newer one did not: the release got there after all. */
  recovered: boolean
  /** How many times composition pinned this version. */
  pins: number
  commit: string
  run?: string
  stages: Stage[]
}

const failed = (s: Stage) => s.state === 'failure' || s.state === 'error'

/** A release's newest stage, by when each was written; the first listed wins a tie, as GitHub lists the newest first. */
const newest = (stages: Stage[]) =>
  stages.reduce<Stage | undefined>(
    (n, s) => (n === undefined || Date.parse(s.at) > Date.parse(n.at) ? s : n),
    undefined,
  )

/** Whether release a is newer than b, both X.Y.Z; false where either is not. */
function newer(a: string, b: string): boolean {
  const release = /^\d+\.\d+\.\d+$/
  return release.test(a) && release.test(b) && a.localeCompare(b, 'en', { numeric: true }) > 0
}

/** Flagger's two settled phases; every other one, and any it adds, is still switching. */
const settled = new Set(['Succeeded', 'Failed'])

/**
 * Where a gated Application stands on the revision Flux applied: the gate's record says best what
 * serves, and Flagger's phases where the record says nothing. A rolled-back Canary is a failure.
 */
function gatedState(gate: Release): ReleaseState {
  const revisions = gate.members.map((m) => m.revision).filter((r) => r !== undefined)
  const record = gate.unreadable === undefined && revisions.length > 0 ? gate.serving : undefined
  if (record !== undefined && revisions.every((r) => r === record)) return 'live'
  if (gate.members.some((m) => m.phase === 'Failed')) return 'failed'
  if (record !== undefined) return 'rolling'
  return gate.members.every((m) => m.phase === 'Succeeded') ? 'live' : 'rolling'
}

/** Where a Project's gated Applications stand together: one failed fails it, one still switching holds it. */
function gatesState(gates: Release[]): ReleaseState {
  const states = gates.filter((g) => g.members.length > 0).map(gatedState)
  if (states.includes('failed')) return 'failed'
  return states.includes('rolling') ? 'rolling' : 'live'
}

/**
 * Each release, newest first. The newest is live once its unit runs it and is ready, and, where the
 * Project's Applications are gated, once what serves is that release: a Kustomization that is Ready
 * says only that Flux applied it. It is overtaken where its unit already runs a newer release that
 * opened no Rollout.
 */
export function releases(rollouts: Rollout[], unit: Unit | undefined, gates: Release[] = []): ReleaseView[] {
  const out: ReleaseView[] = []
  for (const r of rollouts) {
    const previous = out.at(-1)
    if (previous?.version === r.version) {
      previous.pins += 1
      continue
    }
    out.push({
      key: r.id,
      version: r.version,
      at: r.at,
      state: 'past',
      recovered: false,
      pins: 1,
      commit: r.commit,
      ...(r.run ? { run: r.run } : {}),
      stages: r.stages,
    })
  }
  out.forEach((v, i) => {
    const last = newest(v.stages)
    v.recovered = last !== undefined && !failed(last) && v.stages.some(failed)
    if (last !== undefined && failed(last)) v.state = 'failed'
    else if (i === 0 && unit?.version !== undefined && newer(unit.version, v.version)) v.state = 'past'
    else if (i === 0) v.state = unit?.version === v.version && unit.ready ? gatesState(gates) : 'rolling'
  })
  return out
}

/** The release running now: the newest that is live, if any. */
export function live(views: ReleaseView[]): ReleaseView | undefined {
  return views.find((v) => v.state === 'live')
}

/**
 * The release that serves traffic: the live one, or, while a newer one switches in or after it
 * failed, the last that did not fail.
 */
export function serving(views: ReleaseView[]): ReleaseView | undefined {
  const state = views[0]?.state
  if (state !== 'rolling' && state !== 'failed') return live(views)
  return views.slice(1).find((v) => v.state !== 'failed')
}

/** The newest release where it failed and an older one still serves in its place; nothing otherwise. */
export function heldBack(views: ReleaseView[]): HeldBack | undefined {
  const [newest] = views
  if (newest?.state !== 'failed') return undefined
  const older = serving(views)
  return older === undefined ? undefined : { failed: newest.version, serving: older.version }
}

/**
 * Whether a Project has a release in flight: its newest still switching in, a Canary in any phase
 * Flagger has not settled, or a pin the gate saw applied and not yet serving that no Canary failed.
 */
export function inFlight(views: ReleaseView[], gates: Release[]): boolean {
  if (views[0]?.state === 'rolling') return true
  return gates.some(
    (g) =>
      !g.members.some((m) => m.phase === 'Failed') &&
      (g.pinned !== undefined || g.members.some((m) => m.phase !== undefined && !settled.has(m.phase))),
  )
}
