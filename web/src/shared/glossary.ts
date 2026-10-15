/** What each word means. Shown on hover and on keyboard focus. */
export const glossary = {
  // Actions
  pause: 'Hold back new releases. What runs now keeps running.',
  resume: 'Let new releases roll out again.',
  rollBack: 'Go back to the last release that works with the current database. Backs up the database first. The Project stays paused after.',
  repository: 'The code for this Project on GitHub.',
  releaseInFlight: 'A new version is switching in right now. See how far it got.',
  changelog: 'What changed in this release.',
  run: 'The GitHub Actions run that pinned this release.',
  details: 'Show the steps of this release and the containers it ran.',
  logs: 'Show what this container printed.',
  silence: 'Stops notifications for this alert, here and in Discord, for the time you pick.',

  // Facts
  namespace: 'Where this Project runs in the cluster.',
  unit: 'The Flux job that applies this Project. It waits for the units listed after it.',
  pin: 'The exact build the cluster should run. Moving the pin is a deploy.',
  pinHistory: 'Every change to the pin, newest first.',
  estatePath: 'Deployed from its pin in the estate repository.',
  oldPath: 'Still deployed by fleet-infra, the old way.',
  pinnedTwice: 'Pinned more than once with the same version. The build ran again. Nothing new was released.',
  runningNow: 'The containers of the version that runs now.',

  // Health
  composition: 'Whether each Project’s latest release fits in the estate.',
  delivery: 'Whether Flux applied every pin and everything is healthy.',
  releases: 'Versions switching in right now.',
  platform: 'Alerts from the cluster’s monitoring.',

  // Project states
  Live: 'Running its pinned release, healthy.',
  Releasing: 'A new version is switching in.',
  'Refused, isolated': 'Its newest release was rejected. The older one keeps running.',
  Paused: 'New releases are held back. What runs now keeps running.',
  'Rolled back': 'Went back to an older release and is paused.',
  'Not ready': 'Flux applied it, but it isn’t healthy yet.',
  'Not pinned yet': 'On the estate path, but nothing has been deployed yet.',
  'Old path': 'Still deployed by fleet-infra, the old way.',
  Stale: 'No new release published for a while.',

  // Release marks
  live: 'Running now.',
  rolling: 'Switching in now.',
  failed: 'This release failed.',
  past: 'Not running anymore.',

  // Declaration
  switchover: 'How a new version replaces the old one.',
  'blue-green': 'The new version starts next to the old one. Traffic moves over once its checks pass.',
  continuous: 'Pods are replaced one by one, so it never goes down.',
  interrupted: 'The old version stops before the new one starts, so there is a short gap.',
  migration: 'How this Project changes its database between versions.',
  provenAgainst: 'The release this database change was tested with.',
  undo: 'Whether the database change can be reversed if the release is held.',
  grants: 'Secrets each Process is allowed to read or use.',

  // Cluster
  Source: 'Where Flux downloads the pinned build from.',
  Unit: 'The Flux job that applies the build.',
  Canary: 'Watches a new version while it switches in, and stops it if checks fail.',
} as const

export type Term = keyof typeof glossary

/** A word's explanation, or the one for the state it starts with. */
export function explain(word: string): string | undefined {
  if (word in glossary) return glossary[word as Term]
  const prefix = (Object.keys(glossary) as Term[]).find((t) => /^[A-Z]/.test(t) && word.startsWith(t))
  return prefix ? glossary[prefix] : undefined
}
