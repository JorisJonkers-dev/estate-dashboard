import type { Container, ContainerInstance, Update } from '../../../infrastructure/api/types.gen'
import type { Tone } from '../../../shared/tone'

/** The filters over the Containers list, in the order the page shows them. */
export const filters = ['All', 'Needs update', 'Drift', 'Spread', 'Platform'] as const
export type Filter = (typeof filters)[number]

/** The one mark a row carries, where it needs one. */
export interface Mark {
  text: string
  tone: Tone
}

/** One row of the Containers list: an image, its version, what to say of it, and where it runs. */
export interface ContainerRow {
  name: string
  repository: string
  version?: string
  mark?: Mark
  tags: Filter[]
  projects: string[]
  processes: string[]
  newer?: Update
  container: Container
}

/** Compares two versions, their runs of digits as numbers, a leading v ignored. */
export function compareVersions(a: string, b: string): number {
  return a.replace(/^v(?=\d)/, '').localeCompare(b.replace(/^v(?=\d)/, ''), 'en', { numeric: true })
}

/** Each version an image runs now, newest first, with the Processes that run it. */
export function versionsRunning(c: Container): { version: string; processes: string[] }[] {
  const by = new Map<string, Set<string>>()
  for (const i of c.instances) {
    if (i.deleting || i.version === undefined) continue
    const processes = by.get(i.version) ?? new Set<string>()
    processes.add(i.process)
    by.set(i.version, processes)
  }
  return [...by]
    .map(([version, processes]) => ({ version, processes: [...processes].sort() }))
    .sort((a, b) => compareVersions(b.version, a.version))
}

/** The image a Renovate dependency names, by its last part, and its full repository where it has one. */
const lastPart = (name: string) => name.slice(name.lastIndexOf('/') + 1)

/** The open pull request that moves an image past a version, where Renovate has one. */
export function newerThan(name: string, repository: string, version: string | undefined, updates: Update[]) {
  return updates
    .filter((u) => u.dependency === repository || lastPart(u.dependency) === name)
    .filter((u) => version === undefined || compareVersions(u.version, version) > 0)
    .sort((a, b) => compareVersions(b.version, a.version))[0]
}

/** The open pull request that moves an image past the version it runs, where Renovate has one. */
export const newerFor = (c: Container, updates: Update[]) => newerThan(c.name, c.repository, c.version, updates)

/** An image reference's repository, written as the registry knows it, as the containers read does. */
export function repositoryOf(image: string): string {
  const named = image.replace(/@.*$/, '')
  const slash = named.lastIndexOf('/')
  const colon = named.lastIndexOf(':')
  const repository = colon > slash ? named.slice(0, colon) : named
  const [first = '', ...rest] = repository.split('/')
  if (rest.length === 0) return `docker.io/library/${repository}`
  if (!/[.:]/.test(first) && first !== 'localhost') return `docker.io/${repository}`
  return repository
}

/** Whether the image runs at more than one version now. */
export const spread = (c: Container) => versionsRunning(c).length > 1

/** The tag an image names, where every container of it names the same one. */
export const tagOf = (c: Container) => c.instances.find((i) => i.tag !== undefined)?.tag

/** The one mark a row carries: the worst thing to say of it, or none. */
export function markOf(c: Container, newer: Update | undefined): Mark | undefined {
  if (c.drift) return { text: 'drift', tone: 'crit' }
  if (spread(c)) return { text: `${String(versionsRunning(c).length)} versions`, tone: 'warn' }
  if (newer) return { text: `${newer.version} ready`, tone: 'warn' }
  if (c.versionSource === 'tag') return { text: 'from tag', tone: 'idle' }
  if (c.versionSource === 'unknown') return { text: tagOf(c) ? 'no full version' : 'no version', tone: 'idle' }
  return undefined
}

const unique = (values: string[]) => [...new Set(values)].sort()

/** One image as its row: its version, mark, filters and where it runs. */
export function rowOf(c: Container, updates: Update[]): ContainerRow {
  const newer = newerFor(c, updates)
  const tags: Filter[] = []
  if (newer) tags.push('Needs update')
  if (c.drift) tags.push('Drift')
  if (spread(c)) tags.push('Spread')
  if (c.platform) tags.push('Platform')
  const mark = markOf(c, newer)
  return {
    name: c.name,
    repository: c.repository,
    ...(c.version ? { version: c.version } : {}),
    ...(mark ? { mark } : {}),
    tags,
    projects: unique(c.instances.map((i) => i.project)),
    processes: unique(c.instances.map((i) => i.process)),
    ...(newer ? { newer } : {}),
    container: c,
  }
}

/** What a row shows for its version: the version, else the tag it names, else nothing. */
export const shownVersion = (r: ContainerRow) => r.version ?? tagOf(r.container) ?? '—'

/** How many rows each filter keeps. */
export function counts(rows: ContainerRow[]): Record<Filter, number> {
  return Object.fromEntries(
    filters.map((f) => [f, f === 'All' ? rows.length : rows.filter((r) => r.tags.includes(f)).length]),
  ) as Record<Filter, number>
}

/** The rows a filter and a search keep: the search matches an image, a Project or a Process. */
export function keep(rows: ContainerRow[], filter: Filter, query: string): ContainerRow[] {
  const q = query.trim().toLowerCase()
  return rows.filter(
    (r) =>
      (filter === 'All' || r.tags.includes(filter)) &&
      (q === '' ||
        r.name.includes(q) ||
        r.repository.includes(q) ||
        r.projects.some((p) => p.includes(q)) ||
        r.processes.some((p) => p.includes(q))),
  )
}

/** The columns a list sorts by. */
export type SortKey = 'name' | 'version' | 'projects'
/** How a list is sorted: by one column, up or down, or not at all. */
export type Sort = { key: SortKey; dir: 'ascending' | 'descending' } | null

/** A header's click: up, then down, then off; a different column starts up. */
export function cycle(sort: Sort, key: SortKey): Sort {
  if (sort?.key !== key) return { key, dir: 'ascending' }
  return sort.dir === 'ascending' ? { key, dir: 'descending' } : null
}

const sortValue = (r: ContainerRow, key: SortKey) =>
  key === 'name' ? r.name : key === 'version' ? shownVersion(r) : r.projects.join(',')

/** The rows in a sort's order, or as they came where nothing sorts. */
export function sorted(rows: ContainerRow[], sort: Sort): ContainerRow[] {
  if (!sort) return rows
  const sign = sort.dir === 'ascending' ? 1 : -1
  return [...rows].sort((a, b) => sign * compareVersions(sortValue(a, sort.key), sortValue(b, sort.key)))
}

/** Where the version of an instance comes from, in words. */
export function sourceWords(c: Pick<ContainerInstance, 'versionSource'>, tag?: string): string {
  switch (c.versionSource) {
    case 'lock':
      return 'the images lock'
    case 'tag':
      return 'the image tag'
    case 'unknown':
      return tag ? 'nowhere: its tag names no full version' : 'nowhere: it names no version'
  }
}

/** The repository that builds an image: the estate's own by its path, else the one that runs it. */
export function sourceRepository(c: Container, repositoryOf: (project: string) => string | undefined) {
  const own = /^ghcr\.io\/jorisjonkers-dev\/([^/]+)\/[^/]+$/.exec(c.repository)?.[1]
  if (own) return `JorisJonkers-dev/${own}`
  if (!c.repository.startsWith('ghcr.io/jorisjonkers-dev/')) return undefined
  const main = c.instances.find((i) => !i.sidecar && !i.init)
  return main ? repositoryOf(main.project) : undefined
}

/** A short digest, as a fact names it. */
export const short = (digest: string) => digest.replace(/^sha256:/, '').slice(0, 12)
