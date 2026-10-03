import { describe, expect, it } from 'vitest'
import { describeEvent, formatDuration } from './eventText'

describe('formatDuration', () => {
  it('names the two largest units that say something', () => {
    expect(formatDuration(0)).toBe('0 s')
    expect(formatDuration(400)).toBe('0 s')
    expect(formatDuration(45_000)).toBe('45 s')
    expect(formatDuration(30 * 60_000)).toBe('30 min')
    expect(formatDuration(30 * 60_000 + 20_000)).toBe('30 min 20 s')
    expect(formatDuration(2 * 3_600_000 + 5 * 60_000 + 59_000)).toBe('2 h 5 min')
    expect(formatDuration(3_600_000 + 20_000)).toBe('1 h')
    expect(formatDuration(3 * 86_400_000 + 4 * 3_600_000)).toBe('3 d 4 h')
  })

  it('reads a negative span as none', () => {
    expect(formatDuration(-5000)).toBe('0 s')
  })
})

describe('describeEvent', () => {
  it('says an alert fired', () => {
    expect(describeEvent({ status: 'firing', startsAt: '2026-10-03T07:00:00Z' })).toBe('Fired')
  })

  it('says after how long an alert resolved, across offsets', () => {
    expect(describeEvent({ status: 'resolved', startsAt: '2026-10-03T07:00:00Z', endsAt: '2026-10-03T09:30:00+02:00' })).toBe(
      'Resolved after 30 min',
    )
  })

  it('reads a resolved event with no end as fired, which the contract never sends', () => {
    expect(describeEvent({ status: 'resolved', startsAt: '2026-10-03T07:00:00Z' })).toBe('Fired')
  })
})
