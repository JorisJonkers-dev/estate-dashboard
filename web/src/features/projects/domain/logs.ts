import type { Stream } from '../../../infrastructure/api/types.gen'

/** A group of containers on the logs panel. */
export interface StreamGroup {
  name: 'Migrations' | 'Containers'
  streams: Stream[]
}

/** Whether a container is part of a migration Job, by pod or container name. */
export function isMigration(s: Stream): boolean {
  return /migrat/.test(s.pod) || /migrat/.test(s.container)
}

/** Migrations first, then the other containers. Empty groups are left out. */
export function groupStreams(streams: Stream[]): StreamGroup[] {
  const groups: StreamGroup[] = [
    { name: 'Migrations', streams: streams.filter(isMigration) },
    { name: 'Containers', streams: streams.filter((s) => !isMigration(s)) },
  ]
  return groups.filter((g) => g.streams.length > 0)
}

/** A stream's key: its pod and container. */
export const streamKey = (s: Stream) => `${s.pod}/${s.container}`

/** How far back the logs panel can look. */
export const windows = [
  { hours: 1, label: '1 hour' },
  { hours: 6, label: '6 hours' },
  { hours: 24, label: '1 day' },
  { hours: 168, label: '7 days' },
] as const
