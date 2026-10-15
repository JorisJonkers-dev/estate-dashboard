import type { Issue, Release, Source, Stage, Unit } from '../../../infrastructure/api/types.gen'
import type { Tone } from '../../../shared/tone'

/** The Estate repository: where pins, the deploy log and the estate's workflows live. */
export const estateRepository = 'JorisJonkers-dev/estate'

const github = 'https://github.com'

/** Where to go on GitHub for a Project: its repository, its pin, its workflows, a commit. */
export const links = {
  repository: (repository: string) => `${github}/${repository}`,
  deployments: (repository: string) => `${github}/${repository}/deployments/production`,
  commit: (repository: string, sha: string) => `${github}/${repository}/commit/${sha}`,
  pin: (project: string) => `${github}/${estateRepository}/blob/main/projects/${project}/source.yaml`,
  pinHistory: (project: string) => `${github}/${estateRepository}/commits/main/projects/${project}/source.yaml`,
  workflow: (name: 'pause' | 'resume' | 'rollback') => `${github}/${estateRepository}/actions/workflows/${name}.yml`,
  issue: (number: number) => `${github}/${estateRepository}/issues/${String(number)}`,
}

/** How a rollout's stage reads: GitHub's deployment state as a tone. */
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
  const words: Record<string, string> = { success: 'Done', failure: 'Failed', error: 'Failed', inactive: 'Superseded', in_progress: 'Running', queued: 'Queued', pending: 'Pending' }
  return words[stage.state] ?? stage.state
}

/** One thing Flux or Flagger holds for the Project, and what it says. */
export interface ClusterRow {
  kind: 'Source' | 'Unit' | 'Canary'
  name: string
  word: string
  tone: Tone
  detail?: string
}

/** The Project's source, units and Canaries, each with Flux's or Flagger's word on it. */
export function clusterRows(project: string, sources: Source[], units: Unit[], releases: Release[]): ClusterRow[] {
  const rows: ClusterRow[] = []
  for (const s of sources.filter((s) => s.name === `project-${project}`)) {
    rows.push({ kind: 'Source', name: s.name, word: s.ready ? 'Ready' : (s.reason ?? 'Not ready'), tone: s.ready ? 'ok' : 'crit', ...(s.ready || !s.message ? {} : { detail: s.message }) })
  }
  for (const u of units.filter((u) => u.source === `project-${project}`)) {
    rows.push({ kind: 'Unit', name: u.name, word: u.ready ? 'Ready' : (u.reason ?? 'Not ready'), tone: u.ready ? 'ok' : 'crit', ...(u.ready || !u.message ? {} : { detail: u.message }) })
  }
  for (const r of releases.filter((r) => r.namespace === `${project}-system`)) {
    for (const m of r.members) {
      const phase = m.phase ?? 'Not seen yet'
      const tone: Tone = phase === 'Failed' ? 'crit' : phase === 'Succeeded' || phase === 'Initialized' ? 'ok' : m.phase ? 'info' : 'idle'
      rows.push({ kind: 'Canary', name: `${r.namespace}/${m.process}`, word: phase, tone })
    }
  }
  return rows
}

/** The open conditions composition keeps for the Project. */
export function conditionsOf(project: string, issues: Issue[]): Issue[] {
  return issues.filter((i) => i.project === project)
}
