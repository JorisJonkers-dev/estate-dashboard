import type { Stage } from '../../../infrastructure/api/types.gen'
import type { Tone } from '../../../shared/tone'

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
  workflow: (name: 'pause' | 'resume' | 'rollback' | 'retry') =>
    `${github}/${estateRepository}/actions/workflows/${name}.yml`,
  issue: (number: number) => `${github}/${estateRepository}/issues/${String(number)}`,
}

/** A release that failed and was rolled back, and the older one that serves in its place. */
export interface HeldBack {
  failed: string
  serving: string
}

/** What a held-back release says, plainly: "0.21.1 failed and was rolled back; 0.20.0 serves". */
export function heldBackText({ failed, serving }: HeldBack): string {
  return `${failed} failed and was rolled back; ${serving} serves`
}

/**
 * What to do about a held-back release: try it again, or go back. Each is a link to a GitHub
 * workflow a human dispatches; the dashboard runs neither.
 */
export function heldBackActions(held: HeldBack): { label: string; href: string }[] {
  return [
    { label: `Retry ${held.failed}`, href: links.workflow('retry') },
    { label: 'Roll back', href: links.workflow('rollback') },
  ]
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
