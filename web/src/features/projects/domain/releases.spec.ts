import { describe, expect, it } from 'vitest'
import type { Rollout, Unit } from '../../../infrastructure/api/types.gen'
import { live, releases } from './releases'

const rollout = (id: number, version: string, states: string[]): Rollout => ({
  id,
  version,
  commit: `c${String(id)}`,
  at: '2026-10-15T08:00:00Z',
  stages: states.map((state) => ({ state, description: state, at: '2026-10-15T08:00:00Z' })),
  ...(id % 2 ? { run: `https://run/${String(id)}` } : {}),
})
const unit = (version: string, ready = true): Unit => ({ name: 'apps-x', source: 'project-x', path: './', dependsOn: [], ready, version })

describe('the releases of a Project', () => {
  it('folds a version pinned twice into one, and the newest is live once its unit runs it', () => {
    const views = releases([rollout(5, '0.6.0', ['in_progress']), rollout(4, '0.6.0', ['success', 'in_progress']), rollout(3, '0.5.0', ['success'])], unit('0.6.0'))
    expect(views.map((v) => [v.version, v.state, v.pins])).toEqual([
      ['0.6.0', 'live', 2],
      ['0.5.0', 'past', 1],
    ])
    expect(live(views)?.version).toBe('0.6.0')
  })

  it('is rolling until the unit runs it ready, and a failed stage is a failed release', () => {
    expect(releases([rollout(2, '1.1.0', ['in_progress'])], unit('1.0.0')).map((v) => v.state)).toEqual(['rolling'])
    expect(releases([rollout(2, '1.1.0', ['in_progress'])], unit('1.1.0', false)).map((v) => v.state)).toEqual(['rolling'])
    expect(releases([rollout(2, '1.1.0', ['in_progress'])], undefined).map((v) => v.state)).toEqual(['rolling'])
    const views = releases([rollout(3, '1.1.0', ['failure']), rollout(2, '1.0.0', ['error'])], unit('1.0.0'))
    expect(views.map((v) => v.state)).toEqual(['failed', 'failed'])
    expect(live(views)).toBeUndefined()
  })

  it('keeps the run where there is one', () => {
    const [odd, even] = releases([rollout(1, '1.0.0', []), rollout(2, '0.9.0', [])], undefined)
    expect(odd?.run).toBe('https://run/1')
    expect(even).not.toHaveProperty('run')
  })
})
