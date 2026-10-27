import { describe, expect, it } from 'vitest'
import type { Alert, Issue, Release, Unit } from '../../../infrastructure/api/types.gen'
import {
  age,
  describeCondition,
  health,
  needs,
  passes,
  previousRelease,
  projectRows,
  releaseOf,
  type Reads,
} from './overview'

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

  it('never reads a Project as live while Flagger rolled its release back', () => {
    // 2026-10-27: Flux applied the release and went Ready, but the canary failed and the old one serves.
    const reads = base()
    reads.releases = [release('estate-dashboard', 'Failed')]
    const row = projectRows(reads).find((r) => r.name === 'estate-dashboard')
    expect([row?.state, row?.tone]).toEqual(['Release failed, rolled back', 'warn'])
  })

  it('says which release failed and which serves, where it read the release before the newest', () => {
    // 2026-10-27: estate-dashboard 0.21.1 failed its analysis; Flagger rolled back and 0.20.0 serves.
    const reads = base()
    reads.releases = [release('estate-dashboard', 'Failed')]
    reads.units = [unit('delivery', true, '0.6.0'), unit('estate-dashboard', true, '0.21.1')]
    reads.previous = { 'estate-dashboard': '0.20.0', 'delivery': '0.5.0' }
    const rows = projectRows(reads)
    expect(rows.find((r) => r.name === 'estate-dashboard')).toMatchObject({
      state: '0.21.1 failed and was rolled back; 0.20.0 serves',
      tone: 'warn',
      heldBack: { failed: '0.21.1', serving: '0.20.0' },
    })
    expect(rows.find((r) => r.name === 'delivery')).not.toHaveProperty('heldBack')
  })

  it('shows a pinned Project the ledger does not name on the estate path', () => {
    const reads = base()
    reads.handover = { estate: [], legacy: [] }
    expect(projectRows(reads).map((r) => r.name)).toEqual(['delivery', 'estate-dashboard', 'notes'])
  })

  it('reads a pause, a missing pin and missing units, and skips what names no Project', () => {
    const rows = projectRows({
      handover: { estate: ['held', 'fresh', 'empty'], legacy: [] },
      pins: [
        { project: 'held', digest: 'sha256:1', paused: { by: 'joris', at: '2026-10-15T07:00:00Z', reason: 'held' } },
        { project: 'empty', digest: 'sha256:2' },
      ],
      units: [{ ...unit('x'), source: 'estate' }],
      releases: [
        { namespace: 'odd', application: 'odd', members: [{ process: 'o', phase: 'Progressing', iterations: 0 }] },
      ],
      issues: [
        { number: 1, title: 'no project', url: 'u', labels: [], updatedAt: '2026-10-15T08:00:00Z', condition: 'stale' },
        { number: 2, title: 'no condition', url: 'u', labels: [], updatedAt: '2026-10-15T08:00:00Z', project: 'held' },
      ],
      deploys: {},
    })
    expect(rows.map((r) => [r.name, r.state])).toEqual([
      ['empty', 'Not ready'],
      ['fresh', 'Not pinned yet'],
      ['held', 'Paused'],
    ])
    expect(projectRows({ deploys: {} })).toEqual([])
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

  it('warns of an open condition while nothing is isolated, and is all composed with none', () => {
    const reads = base()
    reads.issues = [condition(73, 'grimoire', 'stale'), condition(74, 'notes', 'paused')]
    expect(health(reads)[0]).toMatchObject({ status: '1 condition', tone: 'warn', detail: 'nothing isolated' })
    reads.issues = [condition(74, 'notes', 'paused')]
    expect(health(reads)[0]).toMatchObject({ status: 'All composed', tone: 'ok' })
  })

  it('pages on a page alert, and is calm with nothing in flight', () => {
    const reads = base()
    reads.alerts = [alert('Down', 'page', '2026-10-15T07:00:00Z')]
    reads.releases = [release('a', 'Succeeded'), release('b', 'Failed')]
    expect(
      health(reads)
        .slice(2)
        .map((h) => [h.status, h.tone]),
    ).toEqual([
      ['None in flight', 'warn'],
      ['1 alert', 'crit'],
    ])
    reads.releases = [release('a', 'Succeeded')]
    expect(health(reads)[2]).toMatchObject({ tone: 'ok', detail: 'none failed' })
  })

  it('warns where the Release Gate could not say what a release serves', () => {
    const reads = base()
    reads.releases = [{ ...release('a', 'Failed'), unreadable: 'the Release Gate did not answer: 503' }]
    expect(health(reads)[2]).toMatchObject({
      status: 'None in flight',
      tone: 'warn',
      detail: "1 release failed · the gate's record of 1 release unread",
    })
  })

  it('ranks each alert class by severity', () => {
    const reads = base()
    reads.alerts = [
      alert('Page', 'page', '2026-10-15T07:00:00Z'),
      alert('Urgent', 'urgent', '2026-10-15T07:01:00Z'),
      alert('Later', 'business-hours', '2026-10-15T07:02:00Z'),
    ]
    expect(
      needs(reads)
        .filter((n) => n.class === 'Platform')
        .map((n) => [n.severity, n.tone]),
    ).toEqual([
      ['Info', 'info'],
      ['Warning', 'warn'],
      ['Critical', 'crit'],
    ])
  })

  it('says what it could not read rather than calling it healthy', () => {
    expect(health({ deploys: {} }).map((h) => h.status)).toEqual(['Unknown', 'Unknown', 'Unknown', 'Not read'])
  })
})

describe('when a Project was released', () => {
  it('skips a rewrite of its source and a release of another Project', () => {
    const deploy = (message: string, commit: string) => ({ commit, at: '2026-10-15T08:00:00Z', message })
    const deploys = [
      deploy('chore(deploy): rewrite the pin source of delivery,estate-dashboard [ci skip]', 'r'),
      deploy('chore(deploy): estate-dashboard 0.3.0 [ci skip]', 'd'),
      deploy('chore(deploy): _estate, delivery 0.6.0, estate-dashboard 0.2.0 [ci skip]', 'm'),
    ]
    expect(releaseOf('delivery', deploys)?.commit).toBe('m')
    expect(releaseOf('estate-dashboard', deploys)?.commit).toBe('d')
    expect(releaseOf('dashboard', deploys)).toBeUndefined()
    expect(releaseOf('notes', deploys)).toBeUndefined()
  })

  it('reads the release its pin named before the newest, past a pin that moved again to the same one', () => {
    const deploy = (message: string) => ({ commit: 'c', at: '2026-10-27T08:00:00Z', message })
    const deploys = [
      deploy('chore(deploy): estate-dashboard 0.21.1 [ci skip]'),
      deploy('chore(deploy): rewrite the pin source of estate-dashboard [ci skip]'),
      deploy('chore(deploy): _estate, estate-dashboard 0.21.1 [ci skip]'),
      deploy('chore(deploy): auth 0.21.2, estate-dashboard 0.20.0 [ci skip]'),
      deploy('chore(deploy): estate-dashboard 0.19.0 [ci skip]'),
    ]
    expect(previousRelease('estate-dashboard', deploys)).toBe('0.20.0')
    expect(previousRelease('auth', deploys)).toBeUndefined()
    expect(previousRelease('notes', [])).toBeUndefined()
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

  it('lists a release that failed while an older one serves, with the workflows that retry it or roll back', () => {
    const reads = base()
    reads.releases = [release('estate-dashboard', 'Failed')]
    reads.units = [unit('estate-dashboard', true, '0.21.1')]
    reads.previous = { 'estate-dashboard': '0.20.0' }
    reads.deploys = {
      'estate-dashboard': {
        commit: 'd',
        at: '2026-10-27T08:00:00Z',
        message: 'chore(deploy): estate-dashboard 0.21.1',
      },
    }
    expect(needs(reads)).toEqual([
      {
        key: 'held-estate-dashboard',
        severity: 'Warning',
        tone: 'warn',
        class: 'Release',
        title: 'estate-dashboard 0.21.1 failed and was rolled back; 0.20.0 serves',
        where: 'estate-dashboard',
        since: '2026-10-27T08:00:00Z',
        href: '/projects/estate-dashboard',
        actions: [
          { label: 'Retry 0.21.1', href: 'https://github.com/JorisJonkers-dev/estate/actions/workflows/retry.yml' },
          { label: 'Roll back', href: 'https://github.com/JorisJonkers-dev/estate/actions/workflows/rollback.yml' },
        ],
      },
    ])
    reads.deploys = {}
    expect(needs(reads)).toEqual([])
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
