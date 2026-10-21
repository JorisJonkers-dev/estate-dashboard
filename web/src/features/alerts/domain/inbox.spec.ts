import { describe, expect, it } from 'vitest'
import { inbox } from './inbox'

describe('the Alerts screen', () => {
  it('folds alerts, conditions and releases in flight into one list, newest first', () => {
    const items = inbox(
      [
        {
          fingerprint: 'a',
          name: 'BackupFailed',
          class: 'urgent',
          summary: 'Backup failed',
          startsAt: '2026-10-15T06:00:00Z',
          labels: { alertname: 'BackupFailed', namespace: 'data-system' },
          silencedBy: [],
        },
        {
          fingerprint: 'b',
          name: 'DiskPressure',
          class: 'page',
          startsAt: '2026-10-15T05:00:00Z',
          labels: { alertname: 'DiskPressure' },
          silencedBy: ['s'],
        },
        {
          fingerprint: 'c',
          name: 'Cert',
          class: 'business-hours',
          startsAt: '2026-10-15T01:00:00Z',
          labels: {},
          silencedBy: [],
        },
      ],
      [
        {
          number: 7,
          title: 'knowledge: isolated',
          url: 'https://x/7',
          labels: [],
          updatedAt: '2026-10-15T07:00:00Z',
          project: 'knowledge',
          condition: 'isolated',
        },
        {
          number: 8,
          title: 'notes: paused',
          url: 'https://x/8',
          labels: [],
          updatedAt: '2026-10-15T04:00:00Z',
          project: 'notes',
          condition: 'paused',
        },
        {
          number: 9,
          title: 'grimoire: stale',
          url: 'https://x/9',
          labels: [],
          updatedAt: '2026-10-15T03:00:00Z',
          project: 'grimoire',
          condition: 'stale',
        },
        { number: 5, title: 'Dependency Dashboard', url: 'https://x/5', labels: [], updatedAt: '2026-10-15T09:00:00Z' },
      ],
      [
        {
          namespace: 'auth-system',
          application: 'auth',
          members: [{ process: 'auth-api', phase: 'Progressing', iterations: 2 }],
          pinned: '3.5.0',
          since: '2026-10-15T08:00:00Z',
        },
        {
          namespace: 'quiet-system',
          application: 'quiet',
          members: [{ process: 'q', phase: 'Succeeded', iterations: 4 }],
        },
        {
          namespace: 'new-system',
          application: 'new',
          members: [
            { process: 'n', phase: 'Waiting', iterations: 0 },
            { process: 'm', iterations: 0 },
          ],
        },
      ],
    )
    expect(items.map((i) => [i.class, i.severity, i.title, i.project ?? '', i.silenced])).toEqual([
      ['Releases', 'Info', 'new switching', 'new', false],
      ['Releases', 'Info', 'auth switching to 3.5.0', 'auth', false],
      ['Composition', 'Critical', 'knowledge: isolated', 'knowledge', false],
      ['Platform', 'Warning', 'Backup failed', 'data', false],
      ['Platform', 'Critical', 'DiskPressure', '', true],
      ['Delivery', 'Info', 'notes: paused', 'notes', false],
      ['Composition', 'Warning', 'grimoire: stale', 'grimoire', false],
      ['Platform', 'Info', 'Cert', '', false],
    ])
    expect(items[1]?.link).toEqual({ href: '/projects/auth/release', label: 'Open release' })
    // A member the Release Gate has not seen yet says so.
    expect(items[0]?.labels).toContainEqual(['m', 'not seen, 0 checks'])
  })

  it('reads an alert with no class by its severity label, as the rules on the cluster put it', () => {
    const alert = (fingerprint: string, severity?: string) => ({
      fingerprint,
      name: 'X',
      startsAt: '2026-10-15T06:00:00Z',
      labels: { alertname: 'X', ...(severity ? { severity } : {}) },
      silencedBy: [],
    })
    const items = inbox([alert('c', 'critical'), alert('w', 'warning'), alert('i', 'info'), alert('n')], [], [])
    expect(items.map((i) => [i.fingerprint, i.severity, i.tone])).toEqual([
      ['c', 'Critical', 'crit'],
      ['w', 'Warning', 'warn'],
      ['i', 'Info', 'info'],
      ['n', 'Info', 'info'],
    ])
  })

  it('reads an alert by its class where it has one, whatever its severity label says', () => {
    const items = inbox(
      [
        {
          fingerprint: 'p',
          name: 'X',
          class: 'page',
          startsAt: '2026-10-15T06:00:00Z',
          labels: { severity: 'info' },
          silencedBy: [],
        },
      ],
      [],
      [],
    )
    expect(items[0]?.severity).toBe('Critical')
  })
})
