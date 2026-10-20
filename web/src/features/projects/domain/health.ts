import type { Check } from '../../../infrastructure/api/types.gen'
import type { Health } from './pods'

/** A check that has not probed for this long no longer says much. */
export const staleAfter = 10 * 60_000

/**
 * The checks that probe a Project: its namespace's services, an endpoint named after it, or one
 * named after one of its Processes.
 */
export function checksOf(project: string, processes: string[], checks: Check[]): Check[] {
  const namespace = `.${project}-system`
  return checks.filter((c) => {
    const host = c.host ?? ''
    const inNamespace = host.endsWith(namespace) || host.includes(`${namespace}.svc`)
    return inNamespace || c.name === project || processes.includes(c.name)
  })
}

/** One check's health, from its latest probe and its recent ones. */
export function checkHealth(c: Check, now: number): Health {
  if (c.at === undefined) return { tone: 'idle', word: 'Not probed yet' }
  if (!c.up) return { tone: 'crit', word: 'Down', ...(c.error ? { detail: c.error } : {}) }
  if (now - Date.parse(c.at) > staleAfter) return { tone: 'warn', word: 'Stale', detail: 'Not probed for 10 minutes.' }
  if (c.recent.includes(false)) return { tone: 'warn', word: 'Flapping', detail: 'A recent probe failed.' }
  return { tone: 'ok', word: 'Up' }
}
