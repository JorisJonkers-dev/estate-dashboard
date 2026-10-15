import { describe, expect, it } from 'vitest'
import { byProcess, isMigration, readLine } from './logs'

const s = (pod: string, container: string, version?: string) => ({ pod, container, ...(version ? { version } : {}) })

describe('the containers of a release', () => {
  it('groups one version by Process, migrations first', () => {
    const streams = [s('api-1', 'api', '1.0.0'), s('api-2', 'api', '1.0.0'), s('api-migration-up', 'liquibase', '1.0.0'), s('api-0', 'api', '0.9.0'), s('old', 'api')]
    expect(byProcess(streams, '1.0.0').map((p) => [p.process, p.migration, p.streams.length])).toEqual([
      ['liquibase', true, 1],
      ['api', false, 2],
    ])
    expect(byProcess(streams, undefined).map((p) => p.streams[0]?.pod)).toEqual(['old'])
    expect(isMigration(s('web', 'web'))).toBe(false)
    expect(isMigration(s('x', 'auth-migrate'))).toBe(true)
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
    expect(readLine('ERROR: relation "pages" already exists')).toEqual({ level: 'error', message: 'ERROR: relation "pages" already exists', fields: [] })
    expect(readLine('Warning low disk').level).toBe('warning')
  })

  it('leaves anything else as it is', () => {
    for (const text of ['plain text', '{not json', '{}x', '[1]']) expect(readLine(text).message).toBe(text)
    expect(readLine('null')).toEqual({ message: 'null', fields: [] })
  })
})
