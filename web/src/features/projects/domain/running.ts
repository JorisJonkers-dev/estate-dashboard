import type { Pod, PodContainer, Update } from '../../../infrastructure/api/types.gen'
import type { Tone } from '../../../shared/tone'
import { compareVersions, newerThan, repositoryOf } from '../../containers/domain/containers'
import { podHealth, worst, type Health } from './pods'

/** One container of a Process across its pods: its version, and how many of them have it ready. */
export interface ContainerLine {
  container: string
  repository: string
  version?: string
  /** The version Renovate has ready, where it has one. */
  newer?: string
  ready: number
  total: number
}

/** One Process: its pods, the container that is it, and the sidecars beside it. */
export interface ProcessGroup {
  process: string
  tone: Tone
  ready: number
  total: number
  main: ContainerLine
  sidecars: ContainerLine[]
  pods: Pod[]
}

/** The pods still on a release before the one pinned. */
export interface OlderRelease {
  release: string
  stopping: boolean
  groups: ProcessGroup[]
}

/** The Process a pod is, Flagger's -primary taken off. */
const processOf = (p: Pod) => (p.process || p.name).replace(/-primary$/, '')

function lineOf(name: string, of: (p: Pod) => PodContainer | undefined, pods: Pod[], updates: Update[]): ContainerLine {
  const containers = pods.map(of).filter((c) => c !== undefined)
  const first = containers[0]
  const repository = first ? repositoryOf(first.image) : name
  const version = first?.version
  const newer = newerThan(repository.slice(repository.lastIndexOf('/') + 1), repository, version, updates)
  return {
    container: first?.name ?? name,
    repository,
    ...(version ? { version } : {}),
    ...(newer ? { newer: newer.version } : {}),
    ready: containers.filter((c) => c.ready).length,
    total: containers.length,
  }
}

function groupsOf(pods: Pod[], updates: Update[], now: number): ProcessGroup[] {
  const by = new Map<string, Pod[]>()
  for (const p of pods) by.set(processOf(p), [...(by.get(processOf(p)) ?? []), p])
  return [...by]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([process, of]) => {
      const sidecarNames = [
        ...new Set(of.flatMap((p) => p.containers.filter((c) => c.sidecar).map((c) => c.name))),
      ].sort()
      return {
        process,
        tone: of.map((p) => podHealth(p, now)).reduce<Health>(worst, { tone: 'ok', word: 'Healthy' }).tone,
        ready: of.filter((p) => p.ready).length,
        total: of.length,
        main: lineOf(process, (p) => p.containers.find((c) => !c.sidecar && !c.init), of, updates),
        sidecars: sidecarNames.map((name) =>
          lineOf(name, (p) => p.containers.find((c) => c.name === name), of, updates),
        ),
        pods: of,
      }
    })
}

/**
 * What runs now: each Process of the release pinned, its sidecars under it, and the pods still on
 * an older release apart, newest first. Jobs run to completion and are left out.
 */
export function runningNow(
  pods: Pod[],
  release: string | undefined,
  updates: Update[],
  now: number,
): { current: ProcessGroup[]; older: OlderRelease[] } {
  const serving = pods.filter((p) => p.owner !== 'Job')
  const isCurrent = (p: Pod) => release === undefined || p.version === undefined || p.version === release
  const older = new Map<string, Pod[]>()
  for (const p of serving) {
    if (isCurrent(p)) continue
    // A pod that is not current names a release: one without a label is current.
    const version = String(p.version)
    older.set(version, [...(older.get(version) ?? []), p])
  }
  return {
    current: groupsOf(serving.filter(isCurrent), updates, now),
    older: [...older]
      .sort(([a], [b]) => compareVersions(b, a))
      .map(([version, of]) => ({
        release: version,
        stopping: of.some((p) => p.deleting),
        groups: groupsOf(of, updates, now),
      })),
  }
}
