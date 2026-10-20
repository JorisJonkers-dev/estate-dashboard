import { describe, expect, it } from 'vitest'
import type { Container, ContainerInstance, Update } from '../../../infrastructure/api/types.gen'
import {
  compareVersions,
  counts,
  cycle,
  keep,
  markOf,
  newerFor,
  repositoryOf,
  rowOf,
  short,
  shownVersion,
  sorted,
  sourceRepository,
  sourceWords,
  versionsRunning,
} from './containers'

const instance = (over: Partial<ContainerInstance> = {}): ContainerInstance => ({
  project: 'arr',
  process: 'sonarr',
  container: 'sonarr',
  pod: 'sonarr-1',
  namespace: 'arr-system',
  init: false,
  sidecar: false,
  managed: true,
  versionSource: 'lock',
  drift: false,
  version: '4.0.19',
  ...over,
})

const image = (over: Partial<Container> = {}): Container => ({
  name: 'sonarr',
  repository: 'ghcr.io/jorisjonkers-dev/sonarr',
  version: '4.0.19',
  versionSource: 'lock',
  platform: false,
  drift: false,
  instances: [instance()],
  ...over,
})

const update = (over: Partial<Update> = {}): Update => ({
  dependency: 'linuxserver/sonarr',
  version: '4.0.20',
  repository: 'JorisJonkers-dev/arr',
  number: 27,
  url: 'https://github.com/JorisJonkers-dev/arr/pull/27',
  title: 'update linuxserver/sonarr to 4.0.20',
  opened: '2026-10-20T07:00:00Z',
  ...over,
})

describe('versions', () => {
  it('compare their numbers as numbers, a leading v ignored', () => {
    expect(compareVersions('1.21.10', '1.21.9')).toBeGreaterThan(0)
    expect(compareVersions('v3.7.12', '3.7.12')).toBe(0)
  })

  it('running now are each distinct version, newest first, with the Processes that run it', () => {
    const vault = image({
      instances: [
        instance({ process: 'vault', version: '1.21.2' }),
        instance({ process: 'renew', version: '1.21.4' }),
        instance({ process: 'bootstrap', version: '1.21.4' }),
        instance({ process: 'agent', version: '2.0.4' }),
        instance({ process: 'old', version: '0.1.0', deleting: true }),
        instance({ process: 'nameless', versionSource: 'unknown', version: undefined }),
      ],
    })
    expect(versionsRunning(vault)).toEqual([
      { version: '2.0.4', processes: ['agent'] },
      { version: '1.21.4', processes: ['bootstrap', 'renew'] },
      { version: '1.21.2', processes: ['vault'] },
    ])
  })
})

describe('a newer version', () => {
  it('is the newest open pull request that moves the image past what it runs', () => {
    const newest = update({ version: '4.0.21' })
    expect(newerFor(image(), [update(), newest, update({ version: '4.0.18' })])).toBe(newest)
  })

  it('matches an image by its full repository or by its name', () => {
    const direct = update({ dependency: 'ghcr.io/jorisjonkers-dev/sonarr' })
    expect(newerFor(image(), [direct])).toBe(direct)
    expect(newerFor(image(), [update({ dependency: 'linuxserver/radarr' })])).toBeUndefined()
  })

  it('is any update for an image whose version is unknown', () => {
    expect(newerFor(image({ version: undefined }), [update()])).toBeDefined()
  })
})

describe('a row’s mark', () => {
  it('is drift before anything else', () => {
    expect(markOf(image({ drift: true }), update())).toEqual({ text: 'drift', tone: 'crit' })
  })

  it('counts the versions an image runs at once', () => {
    const c = image({ instances: [instance(), instance({ version: '4.0.18' })] })
    expect(markOf(c, update())).toEqual({ text: '2 versions', tone: 'warn' })
  })

  it('names a newer version that is ready', () => {
    expect(markOf(image(), update())).toEqual({ text: '4.0.20 ready', tone: 'warn' })
  })

  it('says a version only a tag names comes from the tag', () => {
    expect(markOf(image({ versionSource: 'tag' }), undefined)).toEqual({ text: 'from tag', tone: 'idle' })
  })

  it('says a tag that names no full version, or no version at all', () => {
    const pg = image({ versionSource: 'unknown', version: undefined, instances: [instance({ tag: 'pg17' })] })
    expect(markOf(pg, undefined)?.text).toBe('no full version')
    expect(markOf(image({ versionSource: 'unknown', version: undefined }), undefined)?.text).toBe('no version')
  })

  it('is none for an image of the lock that is up to date', () => {
    expect(markOf(image(), undefined)).toBeUndefined()
  })
})

describe('a row', () => {
  it('carries each filter it passes and the Projects and Processes it runs in', () => {
    const r = rowOf(
      image({
        drift: true,
        platform: true,
        instances: [instance({ project: 'b' }), instance({ project: 'a', process: 'x', version: '4.0.18' })],
      }),
      [update()],
    )
    expect(r.tags).toEqual(['Needs update', 'Drift', 'Spread', 'Platform'])
    expect(r.projects).toEqual(['a', 'b'])
    expect(r.processes).toEqual(['sonarr', 'x'])
    expect(r.newer?.number).toBe(27)
  })

  it('shows its version, else its tag, else a dash', () => {
    expect(shownVersion(rowOf(image(), []))).toBe('4.0.19')
    expect(shownVersion(rowOf(image({ version: undefined, instances: [instance({ tag: 'pg17' })] }), []))).toBe('pg17')
    expect(shownVersion(rowOf(image({ version: undefined }), []))).toBe('—')
  })
})

describe('the list', () => {
  const rows = [
    rowOf(
      image({
        name: 'n8n',
        repository: 'ghcr.io/o/n8n',
        version: '2.43.8',
        instances: [instance({ project: 'n8n', process: 'n8n' })],
      }),
      [update({ dependency: 'n8nio/n8n', version: '2.44.0' })],
    ),
    rowOf(
      image({
        name: 'vault',
        repository: 'docker.io/hashicorp/vault',
        version: 'v2.0.4',
        platform: true,
        instances: [instance({ project: 'data', process: 'vault' })],
      }),
      [],
    ),
    rowOf(
      image({
        name: 'alloy',
        repository: 'docker.io/grafana/alloy',
        version: 'v1.19.2',
        instances: [instance({ project: 'observability', process: 'faro' })],
      }),
      [],
    ),
  ]

  it('counts what each filter keeps', () => {
    expect(counts(rows)).toEqual({ 'All': 3, 'Needs update': 1, 'Drift': 0, 'Spread': 0, 'Platform': 1 })
  })

  it('keeps what a filter passes and a search matches by image, Project or Process', () => {
    expect(keep(rows, 'Platform', '').map((r) => r.name)).toEqual(['vault'])
    expect(keep(rows, 'All', 'HASHI').map((r) => r.name)).toEqual(['vault'])
    expect(keep(rows, 'All', 'observ').map((r) => r.name)).toEqual(['alloy'])
    expect(keep(rows, 'All', 'faro').map((r) => r.name)).toEqual(['alloy'])
    expect(keep(rows, 'All', 'n8n').map((r) => r.name)).toEqual(['n8n'])
    expect(keep(rows, 'Drift', '')).toEqual([])
  })

  it('cycles a column up, down, then off, and starts another column up', () => {
    const up = cycle(null, 'name')
    expect(up).toEqual({ key: 'name', dir: 'ascending' })
    const down = cycle(up, 'name')
    expect(down).toEqual({ key: 'name', dir: 'descending' })
    expect(cycle(down, 'name')).toBeNull()
    expect(cycle(down, 'version')).toEqual({ key: 'version', dir: 'ascending' })
  })

  it('sorts by the column a sort names, and leaves the order alone without one', () => {
    expect(sorted(rows, null)).toBe(rows)
    expect(sorted(rows, { key: 'name', dir: 'ascending' }).map((r) => r.name)).toEqual(['alloy', 'n8n', 'vault'])
    expect(sorted(rows, { key: 'version', dir: 'descending' }).map((r) => r.name)).toEqual(['n8n', 'vault', 'alloy'])
    expect(sorted(rows, { key: 'projects', dir: 'ascending' }).map((r) => r.name)).toEqual(['vault', 'n8n', 'alloy'])
  })
})

describe('an image’s facts', () => {
  it('say where its version comes from', () => {
    expect(sourceWords({ versionSource: 'lock' })).toBe('the images lock')
    expect(sourceWords({ versionSource: 'tag' })).toBe('the image tag')
    expect(sourceWords({ versionSource: 'unknown' }, 'pg17')).toBe('nowhere: its tag names no full version')
    expect(sourceWords({ versionSource: 'unknown' })).toBe('nowhere: it names no version')
  })

  it('name the repository that builds it: by its path, by the Project that runs it, or none', () => {
    const repositoryOf = (project: string) => (project === 'arr' ? 'JorisJonkers-dev/arr' : undefined)
    expect(sourceRepository(image({ repository: 'ghcr.io/jorisjonkers-dev/delivery/file-backup' }), repositoryOf)).toBe(
      'JorisJonkers-dev/delivery',
    )
    expect(sourceRepository(image(), repositoryOf)).toBe('JorisJonkers-dev/arr')
    expect(sourceRepository(image({ instances: [instance({ sidecar: true })] }), repositoryOf)).toBeUndefined()
    expect(sourceRepository(image({ repository: 'docker.io/hashicorp/vault' }), repositoryOf)).toBeUndefined()
  })

  it('shorten a digest', () => {
    expect(short('sha256:0123456789abcdef')).toBe('0123456789ab')
  })
})

describe('an image reference', () => {
  it('names its repository the way the registry does', () => {
    expect(repositoryOf('nginx:alpine')).toBe('docker.io/library/nginx')
    expect(repositoryOf('hashicorp/vault:1.21.4')).toBe('docker.io/hashicorp/vault')
    expect(repositoryOf('ghcr.io/o/n8n:v1@sha256:ab')).toBe('ghcr.io/o/n8n')
    expect(repositoryOf('localhost/x')).toBe('localhost/x')
    expect(repositoryOf('registry:5000/x/y')).toBe('registry:5000/x/y')
  })
})
