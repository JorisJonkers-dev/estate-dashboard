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

  it('says what the source pins and what Flux fetched, and what each unit applies', () => {
    const [source, unit] = clusterRows(
      'arr',
      [
        {
          name: 'project-arr',
          url: 'oci://ghcr.io/jorisjonkers-dev/render/arr',
          digest: 'sha256:234dca054d57e22d',
          revision: 'sha256:234dca054d57e22d',
          ready: true,
        },
      ],
      [
        {
          name: 'apps-arr',
          source: 'project-arr',
          path: './apps/arr',
          dependsOn: ['estate-vso-secrets'],
          applied: 'sha256:234dca054d57e22d',
          version: '1.0.4',
          repository: 'JorisJonkers-dev/arr',
          ready: true,
        },
      ],
      [],
    )
    expect(source?.facts).toEqual([
      ['Artifact', 'ghcr.io/jorisjonkers-dev/render/arr'],
      ['Pinned', '234dca054d57'],
      ['Fetched', '234dca054d57'],
    ])
    expect(unit?.facts).toEqual([
      ['Release', '1.0.4 from JorisJonkers-dev/arr'],
      ['Path', './apps/arr'],
      ['Applied', '234dca054d57'],
      ['After', 'estate-vso-secrets'],
    ])
  })

  it('names a release without its repository, and a source or unit Flux has not touched', () => {
    const [source, unit] = clusterRows(
      'arr',
      [{ name: 'project-arr', url: 'oci://r', digest: 'sha256:a', ready: false }],
      [{ name: 'apps-arr', source: 'project-arr', path: './', dependsOn: [], version: '1.0.4', ready: false }],
      [],
    )
    expect(source?.facts).toContainEqual(['Fetched', 'nothing yet'])
    expect(unit?.facts).toEqual([
      ['Release', '1.0.4'],
      ['Path', './'],
      ['Applied', 'nothing yet'],
    ])
  })

  it('says when Flux has not fetched or applied the pin yet', () => {
    const [source, unit] = clusterRows(
      'arr',
      [{ name: 'project-arr', url: 'oci://r', digest: 'sha256:new', revision: 'sha256:old', ready: true }],
      [{ name: 'apps-arr', source: 'project-arr', path: './', dependsOn: [], applied: 'sha256:old', ready: true }],
      [],
    )
    expect(source).toMatchObject({ word: 'Fetching', tone: 'info' })
    expect(unit).toMatchObject({ word: 'Applying', tone: 'info' })
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
