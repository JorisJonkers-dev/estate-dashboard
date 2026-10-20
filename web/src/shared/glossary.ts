/** What each word means. Shown on hover and on keyboard focus. */
export const glossary = {
  // Actions
  'pause': 'Hold back new releases. What runs now keeps running.',
  'resume': 'Let new releases roll out again.',
  'rollBack':
    'Go back to the last release that works with the current database, after backing up the database. The Project stays paused.',
  'repository': 'The code for this Project on GitHub.',
  'changelog': 'What changed in this release.',
  'run': 'The GitHub Actions run that published this release. Not a composition run.',
  'details': 'Show the steps of this release and the containers it ran.',
  'logs': 'Show what this container printed.',
  'silence': 'It stops notifications for this alert, here and in Discord, for the time you pick.',

  // Facts
  'namespace': 'Where this Project runs in the cluster.',
  'unit': 'A Flux job that applies this release. It waits for the units it depends on.',
  'pin': 'The exact release the cluster should run. Moving the pin is a deploy.',
  'pinHistory': 'Every change to the pin, newest first.',
  'estatePath': 'The new way. Deployed from its pin in the estate repository.',
  'oldPath': 'Still deployed by fleet-infra, the old way.',
  'pinnedTwice': 'Pinned more than once with the same release. The workflow ran again. Nothing new was released.',
  'runningNow': 'The containers that run now, as Kubernetes reports them.',
  'Delivery': 'Whether Flux fetched the pinned release and applied it.',
  'Containers': 'How many pods are ready. A Job that ran to completion is not counted.',
  'Online': 'Whether the uptime checks that probe this Project answer.',
  'probes':
    'The checks Kubernetes runs on a container. Readiness decides if it gets traffic; liveness restarts it when it hangs.',
  'restarts': 'How often the container restarted. A restart in the last hour counts as a warning.',
  'releaseLabel': 'The release a pod runs, from its app.kubernetes.io/version label.',
  'check': 'Gatus probes this endpoint every minute. Each bar is one probe, newest on the right.',

  // Health
  'composition': 'Whether each Project’s latest release fits in the estate.',
  'delivery': 'Whether Flux applied every pin and everything is healthy.',
  'releases': 'The releases that are switching in right now.',
  'platform': 'The alerts that come from the cluster’s monitoring.',

  // Project states
  'Live': 'It runs its pinned release and is healthy.',
  'Releasing': 'A new version is switching in.',
  'Refused, isolated': 'Its newest release was refused. The older one keeps running.',
  'Paused': 'New releases are held back. What runs now keeps running.',
  'Rolled back': 'Went back to an older release and is paused.',
  'Not ready': 'Flux applied it, but it isn’t healthy yet.',
  'Not pinned yet': 'On the estate path, but nothing has been deployed yet.',
  'Old path': 'Still deployed by fleet-infra, the old way.',
  'Stale': 'No new release has been published for a while.',

  // Release marks
  'live': 'Running now.',
  'rolling': 'Switching in now.',
  'failed': 'This release failed.',
  'past': 'Not running anymore.',

  // Declaration
  'switchover': 'How a new version replaces the old one.',
  'blue-green': 'The new version starts next to the old one. Traffic moves over once its checks pass.',
  'continuous': 'Pods are replaced one by one, so the Project never goes down.',
  'interrupted': 'The old version stops before the new one starts, so there is a short gap.',
  'migration': 'How this Project changes its database between versions.',
  'provenAgainst': 'The release this database change was tested with.',
  'undo': 'Whether the database change can be reversed if the release is held.',
  'grants': 'Secrets each Process is allowed to read or use.',

  // Cluster
  'Source': 'Where Flux downloads the pinned release from.',
  'Unit': 'A Flux job that applies this release. It waits for the units it depends on.',
  'Canary': 'Watches a new version while it switches in, and stops it if checks fail.',
} as const

export type Term = keyof typeof glossary

/** A word's explanation, or the one for the state it starts with. */
export function explain(word: string): string | undefined {
  if (word in glossary) return glossary[word as Term]
  const prefix = (Object.keys(glossary) as Term[]).find((t) => /^[A-Z]/.test(t) && word.startsWith(t))
  return prefix ? glossary[prefix] : undefined
}
