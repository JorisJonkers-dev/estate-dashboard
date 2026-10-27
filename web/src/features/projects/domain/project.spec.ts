import { describe, expect, it } from 'vitest'
import { heldBackActions, heldBackText, links, stageTone, stageWord } from './project'

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
    expect(links.workflow('retry')).toBe('https://github.com/JorisJonkers-dev/estate/actions/workflows/retry.yml')
    expect(links.issue(72)).toBe('https://github.com/JorisJonkers-dev/estate/issues/72')
  })
})

describe('a release that failed while an older one serves', () => {
  it('says so plainly, and offers to retry it or roll back, each a workflow to open on GitHub', () => {
    const held = { failed: '0.21.1', serving: '0.20.0' }
    expect(heldBackText(held)).toBe('0.21.1 failed and was rolled back; 0.20.0 serves')
    expect(heldBackActions(held)).toEqual([
      { label: 'Retry 0.21.1', href: 'https://github.com/JorisJonkers-dev/estate/actions/workflows/retry.yml' },
      { label: 'Roll back', href: 'https://github.com/JorisJonkers-dev/estate/actions/workflows/rollback.yml' },
    ])
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
