import { describe, expect, it } from 'vitest'
import type { Member, Release, Rollout, Unit } from '../../../infrastructure/api/types.gen'
import { inFlight, live, serving, releases } from './releases'

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

/** A gated Application: its members' Canaries, and what the Release Gate recorded it serves. */
const gated = (members: Partial<Member>[], over: Partial<Release> = {}): Release => ({
  namespace: 'findability-explorer-system',
  application: 'findability-explorer',
  members: members.map((m, i) => ({ process: `p${String(i)}`, iterations: 0, revision: 'sha256:new', ...m })),
  ...over,
})

describe('the releases of a gated Project', () => {
  const incident = [rollout(8, '1.8.0', ['success']), rollout(7, '1.7.0', ['success'])]

  it('is failed, not live, when Flagger rolled the new release back, and the old one still serves', () => {
    // 2026-10-27: Flux applied 1.8.0 and went Ready, the gate's checks failed, Flagger rolled back.
    const gate = gated([{ phase: 'Failed', iterations: 3 }], { serving: 'sha256:old', pinned: 'sha256:new' })
    const views = releases(incident, unit('1.8.0'), [gate])
    expect(views.map((v) => [v.version, v.state])).toEqual([
      ['1.8.0', 'failed'],
      ['1.7.0', 'past'],
    ])
    expect(live(views)).toBeUndefined()
    expect(serving(views)?.version).toBe('1.7.0')
  })

  it('is failed from Flagger alone where the gate answered nothing', () => {
    const gate = gated([{ phase: 'Failed', iterations: 3 }], { unreadable: 'the Release Gate did not answer' })
    expect(releases(incident, unit('1.8.0'), [gate]).map((v) => v.state)).toEqual(['failed', 'past'])
  })

  it('is live once the gate records it serves the revision every Canary carries', () => {
    const gate = gated([{ phase: 'Succeeded' }, { phase: 'Succeeded' }], { serving: 'sha256:new' })
    const views = releases(incident, unit('1.8.0'), [gate])
    expect(views[0]?.state).toBe('live')
    expect(serving(views)?.version).toBe('1.8.0')
  })

  it('takes the gate’s record over Flagger: serving it is live, not serving it yet is still rolling', () => {
    expect(
      releases(incident, unit('1.8.0'), [gated([{ phase: 'Initialized' }], { serving: 'sha256:new' })])[0]?.state,
    ).toBe('live')
    const notYet = gated([{ phase: 'Succeeded' }], { serving: 'sha256:old', pinned: 'sha256:new' })
    expect(releases(incident, unit('1.8.0'), [notYet])[0]?.state).toBe('rolling')
  })

  it('is rolling while any Canary is in any phase but Succeeded or Failed', () => {
    for (const phase of [
      'Initializing',
      'Initialized',
      'Waiting',
      'Progressing',
      'WaitingPromotion',
      'Promoting',
      'Finalising',
      'Terminating',
      undefined,
    ]) {
      const gate = gated([{ phase: 'Succeeded' }, phase === undefined ? {} : { phase }], {
        unreadable: 'the Release Gate did not answer',
      })
      const views = releases(incident, unit('1.8.0'), [gate])
      expect([phase, views[0]?.state]).toEqual([phase, 'rolling'])
      expect(serving(views)?.version).toBe('1.7.0')
    }
  })

  it('falls back to Flagger where the gate’s record says nothing: every Canary Succeeded is live', () => {
    expect(releases(incident, unit('1.8.0'), [gated([{ phase: 'Succeeded' }])])[0]?.state).toBe('live')
    // A record that names no revision a Canary carries cannot say which release serves.
    const unnamed = gated([{ phase: 'Succeeded', revision: undefined }], { serving: 'sha256:new' })
    expect(releases(incident, unit('1.8.0'), [unnamed])[0]?.state).toBe('live')
  })

  it('is live only when every gated Application of the Project is', () => {
    const done = gated([{ phase: 'Succeeded' }], { serving: 'sha256:new' })
    const busy = gated([{ phase: 'Progressing' }], { application: 'other', serving: 'sha256:old' })
    expect(releases(incident, unit('1.8.0'), [done, busy])[0]?.state).toBe('rolling')
    expect(releases(incident, unit('1.8.0'), [busy, gated([{ phase: 'Failed' }])])[0]?.state).toBe('failed')
  })

  it('reads Flagger only once Flux applied the release: before that it is rolling whatever the Canaries say', () => {
    const before = gated([{ phase: 'Failed', revision: 'sha256:old' }], { serving: 'sha256:old' })
    expect(releases(incident, unit('1.7.0'), [before])[0]?.state).toBe('rolling')
    expect(releases(incident, unit('1.8.0', false), [before])[0]?.state).toBe('rolling')
  })

  it('keeps the unit’s rule for an Application with no Canary', () => {
    expect(releases(incident, unit('1.8.0'), [])[0]?.state).toBe('live')
    expect(releases(incident, unit('1.8.0'), [gated([])])[0]?.state).toBe('live')
  })
})

describe('a Project in flight', () => {
  const views = (state: 'live' | 'rolling' | 'failed') =>
    releases(
      [rollout(2, '1.1.0', [state === 'failed' ? 'failure' : 'success'])],
      unit(state === 'rolling' ? '1.0.0' : '1.1.0'),
    )

  it('is one whose newest release is still switching in', () => {
    expect(inFlight(views('rolling'), [])).toBe(true)
    expect(inFlight(views('live'), [])).toBe(false)
    expect(inFlight(views('failed'), [])).toBe(false)
    expect(inFlight([], [])).toBe(false)
  })

  it('is one whose Canary is in any phase but Succeeded or Failed, or whose gate saw a pin not yet serving', () => {
    expect(inFlight(views('live'), [gated([{ phase: 'WaitingPromotion' }])])).toBe(true)
    expect(inFlight(views('live'), [gated([{ phase: 'Succeeded' }], { pinned: 'sha256:next' })])).toBe(true)
    expect(inFlight(views('live'), [gated([{ phase: 'Succeeded' }, { phase: 'Failed' }])])).toBe(false)
    // A rolled-back release the gate holds stays pinned until a new pin lands: it is settled.
    expect(inFlight(views('failed'), [gated([{ phase: 'Failed' }], { pinned: 'sha256:new' })])).toBe(false)
    // A Canary Flagger has not seen yet waits for its first deploy, not for a release in flight.
    expect(inFlight(views('live'), [gated([{}])])).toBe(false)
  })
})
