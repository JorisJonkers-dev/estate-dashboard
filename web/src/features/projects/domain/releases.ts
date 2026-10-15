import type { Rollout, Stage, Unit } from '../../../infrastructure/api/types.gen'

/** Where a release stands now, read from its stages and the unit. */
export type ReleaseState = 'live' | 'rolling' | 'failed' | 'past'

/** One release of a Project, its rollouts of the same version folded into one. */
export interface ReleaseView {
  key: number
  version: string
  at: string
  state: ReleaseState
  /** How many times composition pinned this version: more than one when it was rendered again. */
  pins: number
  commit: string
  run?: string
  stages: Stage[]
}

const failed = (s: Stage) => s.state === 'failure' || s.state === 'error'

/**
 * Each release, newest first. The newest is live once the unit runs its version and is ready,
 * rolling until then; any release with a failed stage failed; the rest are past.
 */
export function releases(rollouts: Rollout[], unit: Unit | undefined): ReleaseView[] {
  const out: ReleaseView[] = []
  for (const r of rollouts) {
    const previous = out.at(-1)
    if (previous?.version === r.version) {
      previous.pins += 1
      continue
    }
    out.push({ key: r.id, version: r.version, at: r.at, state: 'past', pins: 1, commit: r.commit, ...(r.run ? { run: r.run } : {}), stages: r.stages })
  }
  out.forEach((v, i) => {
    if (v.stages.some(failed)) v.state = 'failed'
    else if (i === 0) v.state = unit?.version === v.version && unit.ready ? 'live' : 'rolling'
  })
  return out
}

/** The release running now: the newest that is live, if any. */
export function live(views: ReleaseView[]): ReleaseView | undefined {
  return views.find((v) => v.state === 'live')
}
