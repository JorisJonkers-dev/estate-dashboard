import type { Stream } from '../../../infrastructure/api/types.gen'

/** A Process's containers that wrote logs: one per pod. */
export interface ProcessLogs {
  /** The container's name, which is the Process's. */
  process: string
  migration: boolean
  /** How many of its containers are running. */
  running: number
  streams: Stream[]
}

/** Whether a container belongs to a migration Job, up or down, by its pod's or its own name. */
export function isMigration(s: Stream): boolean {
  return /migrat/.test(s.pod) || /migrat/.test(s.container)
}

/** Running: wrote within 15 minutes. The collector writes every 10. */
export const quietFor = 15 * 60_000

export function isRunning(s: Stream, now: number): boolean {
  return s.last !== undefined && now - Date.parse(s.last) <= quietFor
}

/** One release's containers, by their pods' version label. */
export function ofRelease(streams: Stream[], version: string): Stream[] {
  return streams.filter((s) => s.version === version)
}

/** Containers grouped by Process, migrations first. Each Process's containers are newest first. */
export function byProcess(streams: Stream[], now: number): ProcessLogs[] {
  const out = new Map<string, ProcessLogs>()
  for (const s of streams) {
    const p = out.get(s.container) ?? { process: s.container, migration: isMigration(s), running: 0, streams: [] }
    p.streams.push(s)
    if (isRunning(s, now)) p.running += 1
    out.set(s.container, p)
  }
  for (const p of out.values()) p.streams.sort((a, b) => (b.first ?? '').localeCompare(a.first ?? ''))
  return [...out.values()].sort(
    (a, b) => Number(b.migration) - Number(a.migration) || a.process.localeCompare(b.process),
  )
}

/** A log line: JSON lines split into level, message and fields. */
export interface ReadLine {
  level?: string
  message: string
  fields: [string, string][]
}

const quiet = new Set(['time', 'ts', 'timestamp', 'level', 'lvl', 'msg', 'message'])

/** A plain line's level, if it starts with one. */
const plainLevel = /^(error|warn|warning|info|debug)\b/i

/** Reads one log line. */
export function readLine(text: string): ReadLine {
  if (!text.startsWith('{')) {
    const level = plainLevel.exec(text)?.[1]
    return { ...(level ? { level: level.toLowerCase() } : {}), message: text, fields: [] }
  }
  let o: Record<string, unknown>
  try {
    // A line that starts with { and parses is an object.
    o = JSON.parse(text) as Record<string, unknown>
  } catch {
    return { message: text, fields: [] }
  }
  const word = (v: unknown) => (typeof v === 'string' ? v : JSON.stringify(v))
  const level = o.level ?? o.lvl
  const message = o.msg ?? o.message
  return {
    ...(level === undefined ? {} : { level: word(level).toLowerCase() }),
    message: message === undefined ? '' : word(message),
    fields: Object.entries(o)
      .filter(([k]) => !quiet.has(k))
      .map(([k, v]) => [k, word(v)]),
  }
}

/** How far back to look for logs, in hours. */
export const window = 168
