import { describe, expect, it } from 'vitest'
import { clusterRows, links, stageTone, stageWord } from './project'

const stage = (state: string) => ({ state, description: '', at: '2026-10-15T08:00:00Z' })

describe('a Project on GitHub', () => {
  it('links its repository, deployments, commits, pin, workflows and issues', () => {
    expect(links.repository('JorisJonkers-dev/delivery')).toBe('https://github.com/JorisJonkers-dev/delivery')
    expect(links.deployments('JorisJonkers-dev/delivery')).toBe(
      'https://github.com/JorisJonkers-dev/delivery/deployments/production',
    )
    expect(links.commit('JorisJonkers-dev/delivery', 'abc')).toBe(
      'https://github.com/JorisJonkers-dev/delivery/commit/abc',
    )
    expect(links.pin('delivery')).toBe(
      'https://github.com/JorisJonkers-dev/estate/blob/main/projects/delivery/source.yaml',
    )
    expect(links.pinHistory('delivery')).toBe(
      'https://github.com/JorisJonkers-dev/estate/commits/main/projects/delivery/source.yaml',
    )
    expect(links.workflow('rollback')).toBe('https://github.com/JorisJonkers-dev/estate/actions/workflows/rollback.yml')
    expect(links.issue(72)).toBe('https://github.com/JorisJonkers-dev/estate/issues/72')
  })
})

describe('a rollout stage', () => {
  it('reads as a tone and a word', () => {
    expect(
      ['success', 'failure', 'error', 'inactive', 'in_progress', 'odd'].map((s) => [
        stageTone(stage(s)),
        stageWord(stage(s)),
      ]),
    ).toEqual([
      ['ok', 'Done'],
      ['crit', 'Failed'],
      ['crit', 'Failed'],
      ['idle', 'Replaced'],
      ['info', 'Started'],
      ['info', 'odd'],
    ])
  })
})

describe('what the cluster holds for a Project', () => {
  it('lists its source, units and Canaries with their word, and no one else', () => {
    const rows = clusterRows(
      'auth',
      [
        { name: 'project-auth', url: 'oci://r', digest: 'sha256:1', ready: true },
        { name: 'project-other', url: 'oci://r', digest: 'sha256:2', ready: true },
      ],
      [
        {
          name: 'apps-auth',
          source: 'project-auth',
          path: './',
          dependsOn: [],
          ready: false,
          reason: 'HealthCheckFailed',
          message: 'timed out',
        },
        { name: 'apps-quiet', source: 'project-auth', path: './', dependsOn: [], ready: false },
      ],
      [
        {
          namespace: 'auth-system',
          application: 'auth',
          members: [
            { process: 'auth-api', phase: 'Progressing', iterations: 2 },
            { process: 'auth-ui', iterations: 0 },
            { process: 'auth-old', phase: 'Failed', iterations: 0 },
            { process: 'auth-done', phase: 'Succeeded', iterations: 4 },
          ],
        },
        {
          namespace: 'other-system',
          application: 'other',
          members: [{ process: 'x', phase: 'Failed', iterations: 0 }],
        },
      ],
    )
    expect(rows.map((r) => [r.kind, r.name, r.word, r.tone, r.detail])).toEqual([
      ['Source', 'project-auth', 'Ready', 'ok', undefined],
      ['Unit', 'apps-auth', 'HealthCheckFailed', 'crit', 'timed out'],
      ['Unit', 'apps-quiet', 'Not ready', 'crit', undefined],
      ['Canary', 'auth-system/auth-api', 'Progressing', 'info', undefined],
      ['Canary', 'auth-system/auth-ui', 'Not seen yet', 'idle', undefined],
      ['Canary', 'auth-system/auth-old', 'Failed', 'crit', undefined],
      ['Canary', 'auth-system/auth-done', 'Succeeded', 'ok', undefined],
    ])
  })

  it('names a source that is not ready, and why', () => {
    const [row] = clusterRows(
      'a',
      [
        {
          name: 'project-a',
          url: 'oci://r',
          digest: 'x',
          ready: false,
          reason: 'VerificationError',
          message: 'no signature',
        },
      ],
      [],
      [],
    )
    expect(row).toMatchObject({ word: 'VerificationError', tone: 'crit', detail: 'no signature' })
    const [quiet] = clusterRows('a', [{ name: 'project-a', url: 'oci://r', digest: 'x', ready: false }], [], [])
    expect(quiet).toMatchObject({ word: 'Not ready' })
  })
})
