import type { Stream } from '../../../infrastructure/api/types.gen'

/** A Process's containers that wrote logs: one per pod. */
export interface ProcessLogs {
  /** The container's name, which is the Process's. */
  process: string
  migration: boolean
  streams: Stream[]
}

/** Whether a container belongs to a migration Job, up or down, by its pod's or its own name. */
export function isMigration(s: Stream): boolean {
  return /migrat/.test(s.pod) || /migrat/.test(s.container)
}

/** One release's containers by Process, or unversioned ones when no version is given. */
export function byProcess(streams: Stream[], version: string | undefined): ProcessLogs[] {
  const out = new Map<string, ProcessLogs>()
  for (const s of streams.filter((s) => s.version === version)) {
    const p = out.get(s.container) ?? { process: s.container, migration: isMigration(s), streams: [] }
    p.streams.push(s)
    out.set(s.container, p)
  }
  return [...out.values()].sort((a, b) => Number(b.migration) - Number(a.migration) || a.process.localeCompare(b.process))
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
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch {
    return { message: text, fields: [] }
  }
  if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) return { message: text, fields: [] }
  const o = parsed as Record<string, unknown>
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
