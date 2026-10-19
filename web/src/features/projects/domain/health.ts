import type { Check, Pod, Source, Unit } from '../../../infrastructure/api/types.gen'
import { podHealth, worst, type Health } from './pods'

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

/** One part of a stack's health. */
export interface HealthPart extends Health {
  name: 'Delivery' | 'Containers' | 'Online'
}

/** A stack's health: one word, and the parts it is made of. */
export interface StackHealth extends Health {
  parts: HealthPart[]
}

/** Whether a Flux revision names the digest, with or without a tag before it. */
const names = (revision: string | undefined, digest: string) =>
  revision !== undefined && (revision === digest || revision.endsWith(`@${digest}`))

function delivery(source: Source | undefined, units: Unit[]): HealthPart {
  const part = (h: Health): HealthPart => ({ name: 'Delivery', ...h })
  if (source === undefined) return part({ tone: 'idle', word: 'Not pinned' })
  if (!source.ready)
    return part({
      tone: 'crit',
      word: source.reason ?? 'Not ready',
      ...(source.message ? { detail: source.message } : {}),
    })
  const broken = units.find((u) => !u.ready)
  if (broken)
    return part({
      tone: 'crit',
      word: broken.reason ?? 'Not ready',
      ...(broken.message ? { detail: `${broken.name}: ${broken.message}` } : {}),
    })
  if (!names(source.revision, source.digest)) return part({ tone: 'info', word: 'Fetching the pin' })
  if (units.some((u) => !names(u.applied, source.digest))) return part({ tone: 'info', word: 'Applying the pin' })
  return part({ tone: 'ok', word: 'Applied' })
}

function containers(pods: Pod[], now: number): HealthPart {
  const running = pods.filter((p) => p.owner !== 'Job')
  if (running.length === 0) return { name: 'Containers', tone: 'crit', word: 'Nothing running' }
  const of = running.map((p) => podHealth(p, now)).reduce(worst)
  const ready = running.filter((p) => p.ready).length
  return {
    name: 'Containers',
    tone: of.tone,
    word: `${String(ready)} of ${String(running.length)} ready`,
    ...(of.tone === 'ok' ? {} : { detail: of.word }),
  }
}

function online(checks: Check[], now: number): HealthPart {
  if (checks.length === 0) return { name: 'Online', tone: 'idle', word: 'No checks' }
  const down = checks.filter((c) => c.at !== undefined && !c.up).length
  const of = checks.map((c) => checkHealth(c, now)).reduce(worst)
  const word =
    down > 0
      ? `${String(down)} of ${String(checks.length)} down`
      : `${String(checks.filter((c) => c.up).length)} of ${String(checks.length)} up`
  return { name: 'Online', tone: of.tone, word, ...(of.tone === 'ok' ? {} : { detail: of.word }) }
}

const verdict = { ok: 'Healthy', warn: 'Degraded', crit: 'Down', info: 'Updating', back: 'Updating', idle: 'Unknown' }

/** A stack's health: whether Flux applied its pin, its containers are ready, and its checks answer. */
export function stackHealth(input: {
  source?: Source | undefined
  units: Unit[]
  pods: Pod[]
  checks: Check[]
  now: number
  /** What could not be read: a part not read says so rather than guess. */
  unread?: { pods?: boolean; checks?: boolean }
}): StackHealth {
  const parts: HealthPart[] = [
    delivery(input.source, input.units),
    input.unread?.pods
      ? { name: 'Containers', tone: 'idle', word: 'Not read', detail: 'The cluster did not answer.' }
      : containers(input.pods, input.now),
    input.unread?.checks
      ? { name: 'Online', tone: 'idle', word: 'Not read', detail: 'Gatus did not answer.' }
      : online(input.checks, input.now),
  ]
  const counted = parts.filter((p) => p.tone !== 'idle')
  const of = counted.reduce<Health>(worst, { tone: counted.length === 0 ? 'idle' : 'ok', word: '' })
  return { tone: of.tone, word: verdict[of.tone], parts }
}
