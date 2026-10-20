import type { Release, Source, Stage, Unit } from '../../../infrastructure/api/types.gen'
import type { Tone } from '../../../shared/tone'
import { shortDigest } from '../../overview/domain/overview'

/** The estate repository: pins, the deploy log and workflows. */
export const estateRepository = 'JorisJonkers-dev/estate'

const github = 'https://github.com'

/** Where to go on GitHub for a Project. */
export const links = {
  repository: (repository: string) => `${github}/${repository}`,
  deployments: (repository: string) => `${github}/${repository}/deployments/production`,
  commit: (repository: string, sha: string) => `${github}/${repository}/commit/${sha}`,
  pin: (project: string) => `${github}/${estateRepository}/blob/main/projects/${project}/source.yaml`,
  pinHistory: (project: string) => `${github}/${estateRepository}/commits/main/projects/${project}/source.yaml`,
  workflow: (name: 'pause' | 'resume' | 'rollback') => `${github}/${estateRepository}/actions/workflows/${name}.yml`,
  issue: (number: number) => `${github}/${estateRepository}/issues/${String(number)}`,
}

/** A rollout stage's tone, from GitHub's deployment state. */
export function stageTone(stage: Stage): Tone {
  switch (stage.state) {
    case 'success':
      return 'ok'
    case 'failure':
    case 'error':
      return 'crit'
    case 'inactive':
      return 'idle'
    default:
      return 'info'
  }
}

/** A stage's state in words. */
export function stageWord(stage: Stage): string {
  const words: Record<string, string> = {
    success: 'Done',
    failure: 'Failed',
    error: 'Failed',
    inactive: 'Replaced',
    in_progress: 'Started',
    queued: 'Queued',
    pending: 'Pending',
  }
  return words[stage.state] ?? stage.state
}

/** One thing Flux or Flagger holds for a Project. */
export interface ClusterRow {
  kind: 'Source' | 'Unit' | 'Canary'
  name: string
  word: string
  tone: Tone
  detail?: string
  /** What it holds, as label and value. */
  facts: [string, string][]
}

/** Whether a Flux revision names the digest, with or without a tag before it. */
const names = (revision: string, digest: string) => revision === digest || revision.endsWith(`@${digest}`)
const short = (revision: string) => shortDigest(revision.slice(revision.lastIndexOf('@') + 1))

/** A Project's source, units and Canaries, each with its word. */
export function clusterRows(project: string, sources: Source[], units: Unit[], releases: Release[]): ClusterRow[] {
  const rows: ClusterRow[] = []
  const pinned = sources.find((s) => s.name === `project-${project}`)?.digest
  for (const s of sources.filter((s) => s.name === `project-${project}`)) {
    const fetching = s.ready && s.revision !== undefined && !names(s.revision, s.digest)
    rows.push({
      kind: 'Source',
      name: s.name,
      word: fetching ? 'Fetching' : s.ready ? 'Ready' : (s.reason ?? 'Not ready'),
      tone: fetching ? 'info' : s.ready ? 'ok' : 'crit',
      ...(s.ready || !s.message ? {} : { detail: s.message }),
      facts: [
        ['Artifact', s.url.replace(/^oci:\/\//, '')],
        ['Pinned', shortDigest(s.digest)],
        ['Fetched', s.revision === undefined ? 'nothing yet' : short(s.revision)],
      ],
    })
  }
  for (const u of units.filter((u) => u.source === `project-${project}`)) {
    const applying = u.ready && pinned !== undefined && u.applied !== undefined && !names(u.applied, pinned)
    const facts: [string, string][] = []
    if (u.version) facts.push(['Release', u.repository ? `${u.version} from ${u.repository}` : u.version])
    facts.push(['Path', u.path], ['Applied', u.applied === undefined ? 'nothing yet' : short(u.applied)])
    if (u.dependsOn.length > 0) facts.push(['After', u.dependsOn.join(', ')])
    rows.push({
      kind: 'Unit',
      name: u.name,
      word: applying ? 'Applying' : u.ready ? 'Ready' : (u.reason ?? 'Not ready'),
      tone: applying ? 'info' : u.ready ? 'ok' : 'crit',
      ...(u.ready || !u.message ? {} : { detail: u.message }),
      facts,
    })
  }
  for (const r of releases.filter((r) => r.namespace === `${project}-system`)) {
    for (const m of r.members) {
      const phase = m.phase ?? 'Not seen yet'
      const tone: Tone =
        phase === 'Failed'
          ? 'crit'
          : phase === 'Succeeded' || phase === 'Initialized'
            ? 'ok'
            : m.phase
              ? 'info'
              : 'idle'
      rows.push({
        kind: 'Canary',
        name: `${r.namespace}/${m.process}`,
        word: phase,
        tone,
        facts: m.iterations > 0 ? [['Analysis rounds', String(m.iterations)]] : [],
      })
    }
  }
  return rows
}
