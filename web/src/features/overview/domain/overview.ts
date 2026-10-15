import type { Alert, Deploy, Handover, Issue, Pin, Release, Unit } from '../../../infrastructure/api/types.gen'
import type { Tone } from '../../../shared/tone'

export type { Tone }

/** Which delivery path a Project is on (deploy-kit spec/v1/14-platform-intent.md#handover-ledger). */
export type Path = 'estate' | 'legacy'

/** One row of the Projects table. */
export interface ProjectRow {
  name: string
  path: Path
  state: string
  tone: Tone
  /** The release its unit was pinned at; absent on a legacy Project or one not yet pinned. */
  version?: string
  /** The digest its pin names, shortened; absent likewise. */
  pin?: string
  /** When its pin last moved. */
  deployedAt?: string
}

/** The filters over the Projects table, in the order the page shows them. */
export const filters = ['All', 'Needs attention', 'Live', 'Releasing', 'Paused', 'Refused', 'Old path'] as const
export type Filter = (typeof filters)[number]

/** What the Overview reads, each source absent where it could not be read. */
export interface Reads {
  handover?: Handover
  pins?: Pin[]
  units?: Unit[]
  releases?: Release[]
  issues?: Issue[]
  /** Absent where the dashboard runs without Alertmanager, or it did not answer. */
  alerts?: Alert[]
  /** The newest deploy of each Project, by Project. */
  deploys: Record<string, Deploy | undefined>
}

/** Flagger phases in which a Canary is still switching (Flagger's CanaryPhase). */
const switching = new Set(['Waiting', 'Progressing', 'WaitingPromotion', 'Promoting', 'Finalising'])

/** The condition composition keeps for a pause. The pin already shows it. */
const pausedCondition = 'paused'

/** A Project's name from its unit's source, `project-<project>`. */
export function projectOfUnit(unit: Unit): string | undefined {
  return unit.source.startsWith('project-') ? unit.source.slice('project-'.length) : undefined
}

/** A Project's name from a gated Application's namespace, `<project>-system`. */
export function projectOfRelease(release: Release): string | undefined {
  return release.namespace.endsWith('-system') ? release.namespace.slice(0, -'-system'.length) : undefined
}

/** Whether a gated Application is between a pin and its promotion. */
export function inFlight(release: Release): boolean {
  return release.members.some((m) => m.phase !== undefined && switching.has(m.phase))
}

/** The open conditions composition keeps, other than pauses, by Project. */
function conditionsByProject(issues: Issue[]): Map<string, string[]> {
  const out = new Map<string, string[]>()
  for (const issue of issues) {
    if (!issue.project || !issue.condition || issue.condition === pausedCondition) continue
    out.set(issue.project, [...(out.get(issue.project) ?? []), issue.condition])
  }
  return out
}

/** One Project's state, most pressing first. */
function stateOf(
  path: Path,
  conditions: string[],
  pin: Pin | undefined,
  units: Unit[],
  releasing: boolean,
): { state: string; tone: Tone } {
  if (conditions.includes('isolated')) return { state: 'Refused, isolated', tone: 'crit' }
  const [first] = conditions
  if (first !== undefined) return { state: describeCondition(first), tone: 'warn' }
  if (path === 'legacy') return { state: 'Old path', tone: 'idle' }
  if (pin?.rolledBack) return { state: `Rolled back to ${pin.rolledBack.version}, paused`, tone: 'back' }
  if (pin?.paused) return { state: 'Paused', tone: 'back' }
  if (releasing) return { state: 'Releasing', tone: 'info' }
  if (!pin?.digest) return { state: 'Not pinned yet', tone: 'warn' }
  if (units.length === 0 || units.some((u) => !u.ready)) return { state: 'Not ready', tone: 'warn' }
  return { state: 'Live', tone: 'ok' }
}

/** A condition key in words: `stale-fragment` reads "Stale fragment". */
export function describeCondition(condition: string): string {
  const words = condition.replaceAll('-', ' ')
  return words.charAt(0).toUpperCase() + words.slice(1)
}

/** Every Project in the ledger, plus any pinned Project the ledger leaves out. */
export function projectRows(reads: Reads): ProjectRow[] {
  const pins = new Map((reads.pins ?? []).map((p) => [p.project, p]))
  const conditions = conditionsByProject(reads.issues ?? [])
  const units = new Map<string, Unit[]>()
  for (const unit of reads.units ?? []) {
    const project = projectOfUnit(unit)
    if (project) units.set(project, [...(units.get(project) ?? []), unit])
  }
  const releasing = new Set((reads.releases ?? []).filter(inFlight).map(projectOfRelease))
  const paths = new Map<string, Path>()
  for (const name of reads.handover?.legacy ?? []) paths.set(name, 'legacy')
  for (const name of reads.handover?.estate ?? []) paths.set(name, 'estate')
  // A pin means the estate path, whatever the ledger says. `_estate` is not a Project.
  for (const name of pins.keys()) if (!name.startsWith('_') && !paths.has(name)) paths.set(name, 'estate')

  return [...paths.entries()]
    .map(([name, path]): ProjectRow => {
      const pin = path === 'estate' ? pins.get(name) : undefined
      const own = path === 'estate' ? (units.get(name) ?? []) : []
      const { state, tone } = stateOf(path, conditions.get(name) ?? [], pin, own, releasing.has(name))
      const row: ProjectRow = { name, path, state, tone }
      const version = own.find((u) => u.version)?.version
      if (version) row.version = version
      if (pin?.digest) row.pin = shortDigest(pin.digest)
      const deployed = reads.deploys[name]?.at
      if (path === 'estate' && deployed) row.deployedAt = deployed
      return row
    })
    .sort((a, b) => a.name.localeCompare(b.name))
}

/** The newest pin commit that moved the Project to a release; a rewrite of its source names none. */
export function releaseOf(project: string, deploys: Deploy[]): Deploy | undefined {
  const named = new RegExp(`(^|[\\s:,])${project.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')} v?\\d+\\.\\d+\\.\\d+`)
  return deploys.find((d) => named.test(d.message))
}

/** The first twelve hex digits of a digest. */
export function shortDigest(digest: string): string {
  return digest.replace(/^sha256:/, '').slice(0, 12)
}

/** Whether a row passes a filter. */
export function passes(row: ProjectRow, filter: Filter): boolean {
  switch (filter) {
    case 'All':
      return true
    case 'Needs attention':
      return row.tone === 'crit' || row.tone === 'warn' || row.tone === 'back' || row.tone === 'info'
    case 'Live':
      return row.tone === 'ok'
    case 'Releasing':
      return row.tone === 'info'
    case 'Paused':
      return row.tone === 'back'
    case 'Refused':
      return row.tone === 'crit'
    case 'Old path':
      return row.path === 'legacy'
  }
}

/** One cell of the health strip. */
export interface Health {
  name: 'Composition' | 'Delivery' | 'Releases' | 'Platform'
  status: string
  tone: Tone
  detail: string
}

const plural = (n: number, one: string, many = `${one}s`) => `${String(n)} ${n === 1 ? one : many}`

/** One cell per area, from what was read. */
export function health(reads: Reads): Health[] {
  const composition: Health = (() => {
    if (!reads.issues)
      return { name: 'Composition', status: 'Unknown', tone: 'idle', detail: "couldn't read the estate repository" }
    const isolated = new Set(reads.issues.filter((i) => i.condition === 'isolated').map((i) => i.project))
    const open = reads.issues.filter((i) => i.condition && i.condition !== pausedCondition).length
    if (isolated.size > 0)
      return {
        name: 'Composition',
        status: `${String(isolated.size)} isolated`,
        tone: 'crit',
        detail: plural(open, 'open condition'),
      }
    if (open > 0)
      return { name: 'Composition', status: plural(open, 'condition'), tone: 'warn', detail: 'nothing isolated' }
    return { name: 'Composition', status: 'All composed', tone: 'ok', detail: 'no open condition' }
  })()
  const delivery: Health = (() => {
    if (!reads.units) return { name: 'Delivery', status: 'Unknown', tone: 'idle', detail: "couldn't read the cluster" }
    const ready = reads.units.filter((u) => u.ready).length
    const total = reads.units.length
    const detail = `${String(ready)} of ${plural(total, 'unit')} ready`
    return ready === total
      ? { name: 'Delivery', status: 'Healthy', tone: 'ok', detail }
      : { name: 'Delivery', status: `${String(total - ready)} not ready`, tone: 'crit', detail }
  })()
  const releases: Health = (() => {
    if (!reads.releases)
      return { name: 'Releases', status: 'Unknown', tone: 'idle', detail: "couldn't read the Release Gate" }
    const flying = reads.releases.filter(inFlight).length
    const failed = reads.releases.filter((r) => r.members.some((m) => m.phase === 'Failed')).length
    const detail = failed === 0 ? 'none failed' : `${plural(failed, 'release')} failed`
    if (flying > 0) return { name: 'Releases', status: `${String(flying)} in flight`, tone: 'info', detail }
    return { name: 'Releases', status: 'None in flight', tone: failed > 0 ? 'warn' : 'ok', detail }
  })()
  const platform: Health = (() => {
    if (!reads.alerts)
      return { name: 'Platform', status: 'Not read', tone: 'idle', detail: 'Alertmanager did not answer' }
    const firing = reads.alerts.filter((a) => a.silencedBy.length === 0)
    const silenced = reads.alerts.length - firing.length
    const detail = silenced === 0 ? 'none silenced' : `${String(silenced)} silenced`
    if (firing.length === 0) return { name: 'Platform', status: 'Quiet', tone: 'ok', detail }
    const paging = firing.some((a) => a.class === 'page')
    return { name: 'Platform', status: plural(firing.length, 'alert'), tone: paging ? 'crit' : 'warn', detail }
  })()
  return [composition, delivery, releases, platform]
}

/** One item under Needs you. */
export interface Need {
  key: string
  severity: 'Critical' | 'Warning' | 'Info'
  tone: Tone
  class: 'Composition' | 'Platform'
  title: string
  where: string
  since: string
  /** Where the item is handled: its issue, or the Alerts page. */
  href: string
}

/** Firing alerts and open conditions, newest first. */
export function needs(reads: Reads): Need[] {
  const out: Need[] = []
  for (const issue of reads.issues ?? []) {
    if (!issue.project || !issue.condition || issue.condition === pausedCondition) continue
    const critical = issue.condition === 'isolated'
    out.push({
      key: `issue-${String(issue.number)}`,
      severity: critical ? 'Critical' : 'Warning',
      tone: critical ? 'crit' : 'warn',
      class: 'Composition',
      title: issue.title,
      where: `${issue.project} · estate issue #${String(issue.number)}`,
      since: issue.updatedAt,
      href: issue.url,
    })
  }
  for (const alert of reads.alerts ?? []) {
    if (alert.silencedBy.length > 0) continue
    const severity = alert.class === 'page' ? 'Critical' : alert.class === 'urgent' ? 'Warning' : 'Info'
    out.push({
      key: `alert-${alert.fingerprint}`,
      severity,
      tone: severity === 'Critical' ? 'crit' : severity === 'Warning' ? 'warn' : 'info',
      class: 'Platform',
      title: alert.summary ?? alert.name,
      where: alert.labels.namespace ?? alert.name,
      since: alert.startsAt,
      href: '/alerts',
    })
  }
  return out.sort((a, b) => Date.parse(b.since) - Date.parse(a.since))
}

/** How long ago `at` was: "4 min", "6 h", "2 d". */
export function age(at: string, now: number): string {
  const minutes = Math.max(0, Math.floor((now - Date.parse(at)) / 60_000))
  if (minutes < 1) return 'now'
  if (minutes < 60) return `${String(minutes)} min`
  const hours = Math.floor(minutes / 60)
  if (hours < 48) return `${String(hours)} h`
  return `${String(Math.floor(hours / 24))} d`
}
