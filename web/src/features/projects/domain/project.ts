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
