import { describe, expect, it } from 'vitest'
import type { Release, Source, Unit } from '../../../infrastructure/api/types.gen'
import { deliveryFacts, healthBlocks, probeBlocks } from './delivery'

const source = (over: Partial<Source> = {}): Source => ({
  name: 'project-arr',
  url: 'oci://ghcr.io/jorisjonkers-dev/render/arr',
  digest: 'sha256:234dca054d57aa',
  revision: 'sha256:234dca054d57aa',
  ready: true,
  ...over,
})
const unit = (over: Partial<Unit> = {}): Unit => ({
  name: 'apps-arr',
  source: 'project-arr',
  path: './apps/arr',
  dependsOn: ['estate-vso-secrets'],
  applied: 'latest@sha256:234dca054d57aa',
  version: '1.0.4',
  ready: true,
  ...over,
})
const gated: Release = { namespace: 'arr-system', application: 'arr', members: [{ process: 'sonarr', iterations: 0 }] }

describe('where a Project’s delivery stands', () => {
  it('names its pin and release, that Flux fetched and applied it, after which unit, and its artifact', () => {
    expect(deliveryFacts('arr', [source()], [unit()], [])).toEqual({
      pin: '234dca054d57',
      artifact: 'ghcr.io/jorisjonkers-dev/render/arr',
      release: '1.0.4',
      fetched: { word: 'yes', tone: 'ok' },
      applied: { word: 'yes', tone: 'ok' },
      after: ['estate-vso-secrets'],
      switches: 'continuously, no canary',
    })
  })

  it('says Flux is still fetching or applying a new pin', () => {
    const f = deliveryFacts('arr', [source({ revision: 'sha256:old' })], [unit({ applied: 'sha256:old' })], [])
    expect([f.fetched.word, f.applied.word]).toEqual(['fetching', 'applying'])
  })

  it('names why its source or unit is not ready, or that it is not', () => {
    const named = deliveryFacts(
      'arr',
      [source({ ready: false, reason: 'VerificationError' })],
      [unit({ ready: false, reason: 'HealthCheckFailed' })],
      [],
    )
    expect([named.fetched, named.applied]).toEqual([
      { word: 'VerificationError', tone: 'crit' },
      { word: 'HealthCheckFailed', tone: 'crit' },
    ])
    const quiet = deliveryFacts('arr', [source({ ready: false })], [unit({ ready: false })], [])
    expect([quiet.fetched.word, quiet.applied.word]).toEqual(['not ready', 'not ready'])
  })

  it('says nothing is pinned without a source, and there is no unit without one', () => {
    const f = deliveryFacts('arr', [], [], [])
    expect(f).toEqual({
      fetched: { word: 'nothing pinned', tone: 'idle' },
      applied: { word: 'no unit', tone: 'idle' },
      after: [],
      switches: 'continuously, no canary',
    })
  })

  it('says a unit applies until a source names what it applied, and a gated Project switches through a canary', () => {
    const f = deliveryFacts('arr', [], [unit({ version: undefined })], [gated, { ...gated, namespace: 'other-system' }])
    expect(f.applied.word).toBe('applying')
    expect(f.release).toBeUndefined()
    expect(f.switches).toBe('blue-green, through a canary')
  })
})

describe('a strip', () => {
  it('names each block of the health timeline by its time and state', () => {
    const at = '2026-10-20T11:59:00Z'
    const blocks = healthBlocks(
      {
        step: 30,
        blocks: [
          { at, state: 'warn', detail: 'a pod was not ready' },
          { at, state: 'empty' },
          { at, state: 'ok' },
          { at, state: 'crit' },
        ],
      },
      Date.parse('2026-10-20T12:00:00Z'),
    )
    expect(blocks).toEqual([
      { tone: 'warn', label: '1 min ago · Degraded, a pod was not ready' },
      { tone: 'empty', label: '1 min ago · No data' },
      { tone: 'ok', label: '1 min ago · Healthy' },
      { tone: 'crit', label: '1 min ago · Down' },
    ])
  })

  it('names each of a check’s probes, oldest first', () => {
    expect(probeBlocks([true, false])).toEqual([
      { tone: 'ok', label: '2 probes ago · answered' },
      { tone: 'crit', label: '1 probes ago · failed' },
    ])
  })
})
