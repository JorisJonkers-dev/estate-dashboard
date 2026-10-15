import type { Alert, Issue, Release } from '../../../infrastructure/api/types.gen'
import type { Tone } from '../../../shared/tone'

/** The classes the Alerts screen tabs by. */
export const classes = ['Composition', 'Delivery', 'Releases', 'Platform'] as const
export type AlertClass = (typeof classes)[number]

/** One thing on the Alerts screen: an Alertmanager alert, a condition issue, or a release in flight. */
export interface InboxItem {
  key: string
  class: AlertClass
  severity: 'Critical' | 'Warning' | 'Info'
  tone: Tone
  title: string
  project?: string
  source: string
  since: string
  silenced: boolean
  /** The Alertmanager alert's fingerprint, where it is one: it can be silenced, and has a history. */
  fingerprint?: string
  labels: [string, string][]
  link?: { href: string; label: string }
}

const switching = new Set(['Waiting', 'Progressing', 'WaitingPromotion', 'Promoting', 'Finalising'])

/** Every alert, condition and release in flight, newest first. */
export function inbox(alerts: Alert[], issues: Issue[], releases: Release[]): InboxItem[] {
  const out: InboxItem[] = []
  for (const a of alerts) {
    const severity = a.class === 'page' ? 'Critical' : a.class === 'urgent' ? 'Warning' : 'Info'
    out.push({
      key: `alert-${a.fingerprint}`,
      class: 'Platform',
      severity,
      tone: severity === 'Critical' ? 'crit' : severity === 'Warning' ? 'warn' : 'info',
      title: a.summary ?? a.name,
      ...(a.labels.namespace ? { project: a.labels.namespace.replace(/-system$/, '') } : {}),
      source: 'Alertmanager',
      since: a.startsAt,
      silenced: a.silencedBy.length > 0,
      fingerprint: a.fingerprint,
      labels: Object.entries(a.labels),
    })
  }
  for (const i of issues) {
    if (!i.project || !i.condition) continue
    const paused = i.condition === 'paused' || i.condition === 'held'
    const critical = i.condition === 'isolated'
    out.push({
      key: `issue-${String(i.number)}`,
      class: paused ? 'Delivery' : 'Composition',
      severity: critical ? 'Critical' : paused ? 'Info' : 'Warning',
      tone: critical ? 'crit' : paused ? 'back' : 'warn',
      title: i.title,
      project: i.project,
      source: `estate issue #${String(i.number)}`,
      since: i.updatedAt,
      silenced: false,
      labels: [['condition', i.condition]],
      link: { href: i.url, label: 'Open issue' },
    })
  }
  for (const r of releases) {
    if (!r.members.some((m) => m.phase && switching.has(m.phase))) continue
    const project = r.namespace.replace(/-system$/, '')
    out.push({
      key: `release-${r.namespace}-${r.application}`,
      class: 'Releases',
      severity: 'Info',
      tone: 'info',
      title: `${r.application} switching${r.pinned ? ` to ${r.pinned}` : ''}`,
      project,
      source: 'Release Gate',
      since: r.since ?? new Date().toISOString(),
      silenced: false,
      labels: r.members.map((m) => [m.process, `${m.phase ?? 'not seen'}, ${String(m.iterations)} checks`]),
      link: { href: `/projects/${project}/release`, label: 'Open release' },
    })
  }
  return out.sort((a, b) => Date.parse(b.since) - Date.parse(a.since))
}
