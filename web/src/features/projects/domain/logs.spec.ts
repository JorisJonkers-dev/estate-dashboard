import { describe, expect, it } from 'vitest'
import { byProcess, isMigration, isRunning, ofRelease, readLine } from './logs'

const now = Date.parse('2026-10-15T12:00:00Z')
const at = (minutesAgo: number) => new Date(now - minutesAgo * 60_000).toISOString()
const s = (pod: string, container: string, version?: string, first = 60, last = 1) => ({
  pod,
  container,
  ...(version ? { version } : {}),
  first: at(first),
  last: at(last),
})

describe('the containers of a release', () => {
  it('takes only the ones labelled with its version', () => {
    const streams = [s('api-1', 'api', '1.0.0'), s('api-0', 'api', '0.9.0'), s('unlabelled', 'api')]
    expect(ofRelease(streams, '1.0.0').map((x) => x.pod)).toEqual(['api-1'])
    expect(ofRelease([s('unlabelled', 'api')], '1.0.0')).toEqual([])
  })

  it('groups by Process, migrations first, and counts only the running ones', () => {
    const streams = [
      s('api-1', 'api', '1.0.0', 60, 1),
      s('api-2', 'api', '1.0.0', 30, 2),
      s('api-old', 'api', '1.0.0', 90, 45),
      s('api-migration-up', 'liquibase', '1.0.0', 95, 94),
    ]
    expect(
      byProcess(streams, now).map((p) => [p.process, p.migration, p.running, p.streams.map((x) => x.pod)]),
    ).toEqual([
      ['liquibase', true, 0, ['api-migration-up']],
      ['api', false, 2, ['api-2', 'api-1', 'api-old']],
    ])
    expect(isMigration(s('web', 'web'))).toBe(false)
    expect(isMigration(s('x', 'auth-migrate'))).toBe(true)
  })

  it('reads a container that wrote nothing in 15 minutes as finished', () => {
    expect(isRunning(s('a', 'a', undefined, 60, 15), now)).toBe(true)
    expect(isRunning(s('a', 'a', undefined, 60, 16), now)).toBe(false)
    expect(isRunning({ pod: 'a', container: 'a' }, now)).toBe(false)
  })
})

describe('a log line', () => {
  it('reads a JSON line as its level, message and the other fields', () => {
    expect(readLine('{"time":"t","level":"INFO","msg":"served","path":"/x","ms":3,"ok":true}')).toEqual({
      level: 'info',
      message: 'served',
      fields: [
        ['path', '/x'],
        ['ms', '3'],
        ['ok', 'true'],
      ],
    })
    expect(readLine('{"lvl":"warn","message":"slow"}')).toEqual({ level: 'warn', message: 'slow', fields: [] })
    expect(readLine('{"a":{"b":1}}')).toEqual({ message: '', fields: [['a', '{"b":1}']] })
  })

  it('takes the level a plain line starts with', () => {
    expect(readLine('ERROR: relation "pages" already exists')).toEqual({
      level: 'error',
      message: 'ERROR: relation "pages" already exists',
      fields: [],
    })
    expect(readLine('Warning low disk').level).toBe('warning')
  })

  it('leaves anything else as it is', () => {
    for (const text of ['plain text', '{not json', '{}x', '[1]']) expect(readLine(text).message).toBe(text)
    expect(readLine('null')).toEqual({ message: 'null', fields: [] })
  })
})
