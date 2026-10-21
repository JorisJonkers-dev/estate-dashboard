import { describe, expect, it } from 'vitest'
import type { Rollout, Unit } from '../../../infrastructure/api/types.gen'
import { live, serving, releases } from './releases'

const rollout = (id: number, version: string, states: string[]): Rollout => ({
  id,
  version,
  commit: `c${String(id)}`,
  at: '2026-10-15T08:00:00Z',
  stages: states.map((state) => ({ state, description: state, at: '2026-10-15T08:00:00Z' })),
  ...(id % 2 ? { run: `https://run/${String(id)}` } : {}),
})
const unit = (version: string, ready = true): Unit => ({
  name: 'apps-x',
  source: 'project-x',
  path: './',
  dependsOn: [],
  ready,
  version,
})

describe('the releases of a Project', () => {
  it('folds a version pinned twice into one, and the newest is live once its unit runs it', () => {
    const views = releases(
      [
        rollout(5, '0.6.0', ['in_progress']),
        rollout(4, '0.6.0', ['success', 'in_progress']),
        rollout(3, '0.5.0', ['success']),
      ],
      unit('0.6.0'),
    )
    expect(views.map((v) => [v.version, v.state, v.pins])).toEqual([
      ['0.6.0', 'live', 2],
      ['0.5.0', 'past', 1],
    ])
    expect(live(views)?.version).toBe('0.6.0')
    expect(serving(views)?.version).toBe('0.6.0')
  })

  it('is rolling until the unit runs it ready, and a failed stage is a failed release', () => {
    expect(releases([rollout(2, '1.1.0', ['in_progress'])], unit('1.0.0')).map((v) => v.state)).toEqual(['rolling'])
    expect(releases([rollout(2, '1.1.0', ['in_progress'])], unit('1.1.0', false)).map((v) => v.state)).toEqual([
      'rolling',
    ])
    expect(releases([rollout(2, '1.1.0', ['in_progress'])], undefined).map((v) => v.state)).toEqual(['rolling'])
    const views = releases([rollout(3, '1.1.0', ['failure']), rollout(2, '1.0.0', ['error'])], unit('1.0.0'))
    expect(views.map((v) => v.state)).toEqual(['failed', 'failed'])
    expect(live(views)).toBeUndefined()
  })

  it('is judged by its newest stage, so a release that failed a check and then went ready is live and recovered', () => {
    const stage = (state: string, at: string) => ({ state, description: state, at })
    const jellyfin: Rollout = {
      id: 9,
      version: '10.11.11',
      commit: 'c9',
      at: '2026-10-21T09:12:00Z',
      // Listed oldest first, so only the times say which stage is newest.
      stages: [
        stage('in_progress', '2026-10-21T09:12:00Z'),
        stage('failure', '2026-10-21T09:25:00Z'),
        stage('success', '2026-10-21T09:32:00Z'),
      ],
    }
    const [view] = releases([jellyfin], unit('10.11.11'))
    expect([view?.state, view?.recovered]).toEqual(['live', true])
  })

  it('is failed when its newest stage failed, wherever the stage is listed', () => {
    const stage = (state: string, at: string) => ({ state, description: state, at })
    const listedOldestFirst: Rollout = {
      id: 9,
      version: '1.1.0',
      commit: 'c9',
      at: '2026-10-21T09:12:00Z',
      stages: [stage('in_progress', '2026-10-21T09:12:00Z'), stage('failure', '2026-10-21T09:25:00Z')],
    }
    expect(releases([listedOldestFirst], unit('1.0.0'))[0]?.state).toBe('failed')
  })

  it('is not recovered when no stage failed, nor when its newest stage did', () => {
    expect(releases([rollout(2, '1.1.0', ['success', 'in_progress'])], unit('1.1.0'))[0]?.recovered).toBe(false)
    expect(releases([rollout(2, '1.1.0', ['failure', 'success'])], unit('1.0.0'))[0]?.recovered).toBe(false)
  })

  it('is overtaken, not switching in, when its unit already runs a newer release', () => {
    expect(releases([rollout(2, '10.11.11', ['success'])], unit('10.12.0')).map((v) => v.state)).toEqual(['past'])
    expect(releases([rollout(2, '1.9.0', ['success'])], unit('1.10.0')).map((v) => v.state)).toEqual(['past'])
  })

  it('still switches in while its unit runs an older release, or one that is no version', () => {
    expect(releases([rollout(2, '1.10.0', ['in_progress'])], unit('1.9.0')).map((v) => v.state)).toEqual(['rolling'])
    expect(releases([rollout(2, '1.1.0', ['in_progress'])], unit('main')).map((v) => v.state)).toEqual(['rolling'])
  })

  it('is served by the last good release while a newer one switches in', () => {
    const views = releases(
      [rollout(4, '1.2.0', ['in_progress']), rollout(3, '1.1.0', ['failure']), rollout(2, '1.0.0', [])],
      unit('1.0.0'),
    )
    expect(serving(views)?.version).toBe('1.0.0')
    expect(serving(releases([rollout(2, '1.1.0', ['in_progress'])], unit('1.0.0')))).toBeUndefined()
  })

  it('keeps the run where there is one', () => {
    const [odd, even] = releases([rollout(1, '1.0.0', []), rollout(2, '0.9.0', [])], undefined)
    expect(odd?.run).toBe('https://run/1')
    expect(even).not.toHaveProperty('run')
  })
})
