import type { Stream } from '../../../infrastructure/api/types.gen'

/** A group of containers on the logs panel. */
export interface StreamGroup {
  name: 'Migrations' | 'Containers'
  streams: Stream[]
}

/** Whether a container belongs to a migration Job, up or down, by its pod's or its own name. */
export function isMigration(s: Stream): boolean {
  return /migrat/.test(s.pod) || /migrat/.test(s.container)
}

/** Migrations first, then every other container; a group with nothing in it is left out. */
export function groupStreams(streams: Stream[]): StreamGroup[] {
  const groups: StreamGroup[] = [
    { name: 'Migrations', streams: streams.filter(isMigration) },
    { name: 'Containers', streams: streams.filter((s) => !isMigration(s)) },
  ]
  return groups.filter((g) => g.streams.length > 0)
}

/** A stream's key: pod and container, which together name one container. */
export const streamKey = (s: Stream) => `${s.pod}/${s.container}`

/** How far back the panel can look, in hours, with how it reads. */
export const windows = [
  { hours: 1, label: '1 hour' },
  { hours: 6, label: '6 hours' },
  { hours: 24, label: '1 day' },
  { hours: 168, label: '7 days' },
] as const
