import type { Pod, PodContainer } from '../../../infrastructure/api/types.gen'
import type { Tone } from '../../../shared/tone'

/** A health word, its tone, and what to say on hover. */
export interface Health {
  tone: Tone
  word: string
  detail?: string
}

/** A restart this recent still counts against a container. */
export const restartWindow = 60 * 60_000

/** The reasons a waiting container will not get better by itself. */
const stuck = /BackOff|ErrImage|InvalidImage|CreateContainer|RunContainerError/

const rank: Record<Tone, number> = { crit: 5, warn: 4, info: 3, back: 2, idle: 1, ok: 0 }

/** The worse of two healths. */
export function worst(a: Health, b: Health): Health {
  return rank[b.tone] > rank[a.tone] ? b : a
}

/** One container's health, from what the kubelet says of it. */
export function containerHealth(c: PodContainer, now: number): Health {
  if (c.state === 'waiting') {
    const word = c.reason ?? 'Waiting'
    return {
      tone: c.reason && stuck.test(c.reason) ? 'crit' : 'info',
      word,
      ...(c.message ? { detail: c.message } : {}),
    }
  }
  if (c.state === 'terminated') {
    if (c.init && c.reason === 'Completed') return { tone: 'idle', word: 'Done' }
    return { tone: c.reason === 'Completed' ? 'idle' : 'crit', word: c.reason ?? 'Stopped' }
  }
  if (c.state !== 'running') return { tone: 'info', word: 'Starting' }
  if (!c.ready && !c.init) return { tone: 'warn', word: 'Not ready', detail: 'Its readiness check fails.' }
  const last = c.last
  if (last?.at !== undefined && now - Date.parse(last.at) <= restartWindow) {
    return {
      tone: 'warn',
      word: 'Restarted',
      detail: `Restarted ${String(c.restarts)}×; it last ended ${last.reason ?? 'with an error'} (exit ${String(last.exitCode)}).`,
    }
  }
  return { tone: 'ok', word: 'Healthy' }
}

/** One pod's health: its phase, then the worst of its containers. */
export function podHealth(p: Pod, now: number): Health {
  if (p.deleting) return { tone: 'idle', word: 'Stopping' }
  if (p.phase === 'Failed') return { tone: 'crit', word: 'Failed' }
  if (p.phase === 'Succeeded') return { tone: 'idle', word: 'Finished' }
  if (p.phase === 'Pending' && p.containers.every((c) => c.state === undefined))
    return { tone: 'info', word: 'Pending' }
  const healths = p.containers.map((c) => containerHealth(c, now))
  const of = healths.reduce<Health>(worst, { tone: 'ok', word: 'Healthy' })
  if (of.tone === 'ok' && !p.ready)
    return { tone: 'warn', word: 'Not ready', detail: 'Kubernetes does not send it traffic.' }
  return of
}

/** One Process's pods, and how many of them are ready. */
export interface ProcessPods {
  process: string
  /** A Job's pods: a migration, a backup or another run to completion. */
  job: boolean
  ready: number
  pods: Pod[]
}

/** A Project's pods by Process: long-running ones by name, then Jobs. */
export function processes(pods: Pod[]): ProcessPods[] {
  const out = new Map<string, ProcessPods>()
  for (const p of pods) {
    // Flagger names its primary's pods after the Process with -primary added.
    const named = p.process || p.name
    const process = p.name.includes('-primary-') ? named.replace(/-primary$/, '') : named
    const job = p.owner === 'Job'
    const key = `${String(job)}/${process}`
    const group = out.get(key) ?? { process, job, ready: 0, pods: [] }
    group.pods.push(p)
    if (p.ready) group.ready += 1
    out.set(key, group)
  }
  return [...out.values()].sort((a, b) => Number(a.job) - Number(b.job) || a.process.localeCompare(b.process))
}

/** A pod's name in a list of its Process's pods: Flagger's role, or the pod's own suffix. */
export function podRole(p: Pod, all: Pod[]): string {
  if (all.some((o) => o.name.includes('-primary-'))) return p.name.includes('-primary-') ? 'primary' : 'canary'
  return p.name.slice(p.name.lastIndexOf('-') + 1)
}
