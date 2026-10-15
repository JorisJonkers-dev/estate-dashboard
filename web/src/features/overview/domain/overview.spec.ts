import { describe, expect, it } from 'vitest'
import type { Alert, Issue, Release, Unit } from '../../../infrastructure/api/types.gen'
import { age, describeCondition, health, needs, passes, projectRows, type Reads } from './overview'

const unit = (project: string, ready = true, version?: string): Unit => ({
  name: `apps-${project}`,
  source: `project-${project}`,
  path: `./apps/${project}`,
  dependsOn: [],
  ready,
  ...(version ? { version } : {}),
})

const condition = (number: number, project: string, kind: string, updatedAt = '2026-10-15T08:00:00Z'): Issue => ({
  number,
  title: `${project}: ${kind}`,
  url: `https://github.com/JorisJonkers-dev/estate/issues/${String(number)}`,
  labels: ['area: deploy'],
  updatedAt,
  project,
  condition: kind,
})

const release = (project: string, phase: string): Release => ({
  namespace: `${project}-system`,
  application: project,
  members: [{ process: project, phase, iterations: 1 }],
})

const alert = (name: string, cls: Alert['class'], startsAt: string, silencedBy: string[] = []): Alert => ({
  fingerprint: name,
  name,
  startsAt,
  labels: { alertname: name, namespace: 'data-system' },
  silencedBy,
  ...(cls ? { class: cls } : {}),
})

const base = (): Reads => ({
  handover: { estate: ['delivery', 'estate-dashboard', 'notes'], legacy: ['data', 'auth'] },
  pins: [
    { project: '_estate', digest: 'sha256:e' },
    { project: 'delivery', digest: 'sha256:8a77a093432ae80d0d79' },
    { project: 'estate-dashboard', digest: 'sha256:1' },
    {
      project: 'notes',
      digest: 'sha256:2',
      paused: { by: 'joris', at: '2026-10-15T07:00:00Z', reason: 'held' },
      rolledBack: { version: '0.9.1', fragment: 'sha256:f' },
    },
  ],
  units: [unit('delivery', true, '0.6.0'), unit('estate-dashboard', true, '0.2.0'), unit('notes', true, '0.9.1')],
  releases: [],
  issues: [],
  alerts: [],
  deploys: { delivery: { commit: 'a', at: '2026-10-15T06:00:00Z', message: 'chore(deploy): delivery 0.6.0' } },
})

describe('the Projects table', () => {
  it('lists every Project in the ledger by name, legacy ones on the old path, and never the estate pin', () => {
    const rows = projectRows(base())
    expect(rows.map((r) => [r.name, r.path, r.state])).toEqual([
      ['auth', 'legacy', 'Old path'],
      ['data', 'legacy', 'Old path'],
      ['delivery', 'estate', 'Live'],
      ['estate-dashboard', 'estate', 'Live'],
      ['notes', 'estate', 'Rolled back to 0.9.1, paused'],
    ])
    const delivery = rows.find((r) => r.name === 'delivery')
    expect(delivery).toMatchObject({
      version: '0.6.0',
      pin: '8a77a093432a',
      deployedAt: '2026-10-15T06:00:00Z',
      tone: 'ok',
    })
    expect(rows.find((r) => r.name === 'auth')).not.toHaveProperty('version')
  })

  it('puts a refusal before a pause, a release before readiness, and says what is not ready', () => {
    const reads = base()
    reads.issues = [condition(72, 'notes', 'isolated'), condition(73, 'data', 'missing')]
    reads.releases = [release('estate-dashboard', 'Progressing')]
    reads.units = [unit('delivery', false, '0.6.0'), unit('estate-dashboard', true, '0.2.0'), unit('notes', true)]
    const states = Object.fromEntries(projectRows(reads).map((r) => [r.name, [r.state, r.tone]]))
    expect(states).toMatchObject({
      'notes': ['Refused, isolated', 'crit'],
      'data': ['Missing', 'warn'],
      'estate-dashboard': ['Releasing', 'info'],
      'delivery': ['Not ready', 'warn'],
    })
  })

  it('shows a pinned Project the ledger does not name on the estate path', () => {
    const reads = base()
    reads.handover = { estate: [], legacy: [] }
    expect(projectRows(reads).map((r) => r.name)).toEqual(['delivery', 'estate-dashboard', 'notes'])
  })

  it('filters by state, and a pause or a release needs attention', () => {
    const rows = projectRows(base())
    expect(rows.filter((r) => passes(r, 'Needs attention')).map((r) => r.name)).toEqual(['notes'])
    expect(rows.filter((r) => passes(r, 'Old path')).map((r) => r.name)).toEqual(['auth', 'data'])
    expect(rows.filter((r) => passes(r, 'Paused')).map((r) => r.name)).toEqual(['notes'])
  })
})

describe('the health strip', () => {
  it('answers each class from what was read', () => {
    const reads = base()
    reads.issues = [condition(72, 'notes', 'isolated'), condition(74, 'notes', 'paused')]
    reads.releases = [release('estate-dashboard', 'Promoting'), release('delivery', 'Failed')]
    reads.alerts = [
      alert('BackupFailed', 'urgent', '2026-10-15T07:00:00Z'),
      alert('Old', 'page', '2026-10-15T07:00:00Z', ['s-1']),
    ]
    expect(health(reads).map((h) => [h.name, h.status, h.tone, h.detail])).toEqual([
      ['Composition', '1 isolated', 'crit', '1 open condition'],
      ['Delivery', 'Healthy', 'ok', '3 of 3 units ready'],
      ['Releases', '1 in flight', 'info', '1 release failed'],
      ['Platform', '1 alert', 'warn', '1 silenced'],
    ])
  })

  it('says what it could not read rather than calling it healthy', () => {
    expect(health({ deploys: {} }).map((h) => h.status)).toEqual(['Unknown', 'Unknown', 'Unknown', 'Not read'])
  })
})

describe('what needs you', () => {
  it('lists conditions and unsilenced alerts, newest first, and never a pause', () => {
    const reads = base()
    reads.issues = [condition(72, 'notes', 'isolated', '2026-10-15T06:00:00Z'), condition(74, 'notes', 'paused')]
    reads.alerts = [
      alert('BackupFailed', 'urgent', '2026-10-15T07:00:00Z'),
      alert('Quiet', 'page', '2026-10-15T09:00:00Z', ['s']),
    ]
    expect(needs(reads).map((n) => [n.severity, n.class, n.title, n.href])).toEqual([
      ['Warning', 'Platform', 'BackupFailed', '/alerts'],
      ['Critical', 'Composition', 'notes: isolated', 'https://github.com/JorisJonkers-dev/estate/issues/72'],
    ])
  })
})

describe('the words', () => {
  it('writes a condition and an age as the canvas does', () => {
    expect(describeCondition('stale-fragment')).toBe('Stale fragment')
    const now = Date.parse('2026-10-15T10:00:00Z')
    expect(
      ['2026-10-15T10:00:00Z', '2026-10-15T09:56:00Z', '2026-10-15T04:00:00Z', '2026-10-12T10:00:00Z'].map((at) =>
        age(at, now),
      ),
    ).toEqual(['now', '4 min', '6 h', '3 d'])
  })
})
