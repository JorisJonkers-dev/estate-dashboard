import type { Rollout, Stage, Unit } from '../../../infrastructure/api/types.gen'

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

/** Each release, newest first. The newest is live once its unit runs it and is ready. */
export function releases(rollouts: Rollout[], unit: Unit | undefined): ReleaseView[] {
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
    else if (i === 0) v.state = unit?.version === v.version && unit.ready ? 'live' : 'rolling'
  })
  return out
}

/** The release running now: the newest that is live, if any. */
export function live(views: ReleaseView[]): ReleaseView | undefined {
  return views.find((v) => v.state === 'live')
}

/** The release that serves traffic: the live one, or, while a newer one switches in, the last that did not fail. */
export function serving(views: ReleaseView[]): ReleaseView | undefined {
  if (views[0]?.state !== 'rolling') return live(views)
  return views.slice(1).find((v) => v.state !== 'failed')
}
