/**
 * A made-up estate for working on the design: every state a screen can show, at once. Loaded only
 * when the dev server runs with VITE_MOCK=1, so no build ships it.
 */

const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString()
const run = (id: number) => `https://github.com/JorisJonkers-dev/estate/actions/runs/${String(id)}`

const estate = ['auth', 'delivery', 'estate-dashboard', 'knowledge', 'notes']
const legacy = ['agents', 'data', 'grimoire', 'mail', 'media', 'tribelt']

const unit = (project: string, version: string, repository: string, ready = true, reason?: string, message?: string) => ({
  name: `apps-${project}`,
  source: `project-${project}`,
  path: `./apps/${project}`,
  dependsOn: project === 'delivery' ? ['estate-vso-secrets'] : ['estate-vso-secrets', 'apps-delivery'],
  applied: 'sha256:1f2e',
  version,
  repository,
  ready,
  ...(reason ? { reason } : {}),
  ...(message ? { message } : {}),
})

const rollout = (id: number, version: string, at: number, stages: [string, string, number][]) => ({
  id,
  version,
  commit: `${String(id)}a9c3e7b2d41f0e8c6a5b3d2e1f0a9b8c7d6e5f4a`.slice(0, 40),
  run: run(id),
  artifact: `ghcr.io/jorisjonkers-dev/render/x@sha256:${String(id)}`,
  at: minutesAgo(at),
  stages: stages.map(([state, description, m]) => ({ state, description, log: run(id), at: minutesAgo(m) })),
})

const answers: Record<string, unknown> = {
  '/api/v1/session': { subject: 'user-1', name: 'joris' },
  '/api/v1/estate/handover': { estate, legacy },
  '/api/v1/estate/pins': {
    items: [
      { project: '_estate', digest: 'sha256:6201b5a9' },
      { project: 'auth', digest: 'sha256:5c7e9a1d3f22' },
      { project: 'delivery', digest: 'sha256:c26c5e74fcb0' },
      { project: 'estate-dashboard', digest: 'sha256:3a178f470d79' },
      { project: 'knowledge', digest: 'sha256:1ad39d5e7c10' },
      {
        project: 'notes',
        digest: 'sha256:79e5ee9b4a21',
        paused: { by: 'joris', at: minutesAgo(62), reason: 'rolled back after a bad migration' },
        rolledBack: { version: '0.9.1', fragment: 'sha256:f' },
      },
    ],
  },
  '/api/v1/delivery/units': {
    truncated: false,
    items: [
      unit('auth', '3.4.2', 'JorisJonkers-dev/auth-api'),
      unit('delivery', '0.6.0', 'JorisJonkers-dev/delivery'),
      unit('estate-dashboard', '0.2.0', 'JorisJonkers-dev/estate-dashboard'),
      unit('knowledge', '1.12.0', 'JorisJonkers-dev/knowledge', false, 'HealthCheckFailed', 'Deployment knowledge-system/knowledge-api is not ready: 0 of 2 replicas'),
      unit('notes', '0.9.1', 'JorisJonkers-dev/notes'),
    ],
  },
  '/api/v1/delivery/sources': {
    truncated: false,
    items: estate.map((p) => ({ name: `project-${p}`, url: `oci://ghcr.io/jorisjonkers-dev/render/${p}`, digest: 'sha256:1', revision: 'sha256:1', ready: true, reason: 'Succeeded' })),
  },
  '/api/v1/delivery/releases': {
    truncated: false,
    items: [
      {
        namespace: 'auth-system',
        application: 'auth',
        members: [
          { process: 'auth-api', phase: 'Progressing', iterations: 2 },
          { process: 'auth-ui', phase: 'Progressing', iterations: 2 },
        ],
        migration: { identity: 'auth-migration', testedAgainst: '3.4.2', nonTransactional: false },
        serving: '3.4.2',
        pinned: '3.5.0',
        since: minutesAgo(4),
      },
      { namespace: 'estate-dashboard-system', application: 'estate-dashboard', members: [{ process: 'estate-dashboard', phase: 'Succeeded', iterations: 4 }], serving: '0.2.0' },
    ],
  },
  '/api/v1/estate/issues': {
    items: [
      {
        number: 212,
        title: 'knowledge: 1.13.1 isolated',
        url: 'https://github.com/JorisJonkers-dev/estate/issues/212',
        labels: ['area: deploy'],
        updatedAt: minutesAgo(2880),
        project: 'knowledge',
        condition: 'isolated',
      },
      {
        number: 205,
        title: 'grimoire: fragment is stale',
        url: 'https://github.com/JorisJonkers-dev/estate/issues/205',
        labels: ['area: deploy'],
        updatedAt: minutesAgo(12960),
        project: 'grimoire',
        condition: 'stale',
      },
      { number: 5, title: 'Dependency Dashboard', url: 'https://github.com/JorisJonkers-dev/estate/issues/5', labels: [], updatedAt: minutesAgo(30) },
    ],
  },
  '/api/v1/alerts': {
    items: [
      {
        fingerprint: 'a1',
        name: 'BackupFailed',
        class: 'urgent',
        summary: 'Backup failed: rabbitmq-data-backup',
        startsAt: minutesAgo(360),
        labels: { alertname: 'BackupFailed', namespace: 'data-system' },
        silencedBy: [],
      },
      {
        fingerprint: 'a2',
        name: 'CertificateExpiring',
        class: 'business-hours',
        summary: 'auth.jorisjonkers.dev expires in 12 days',
        startsAt: minutesAgo(1500),
        labels: { alertname: 'CertificateExpiring', namespace: 'edge-system' },
        silencedBy: [],
      },
      { fingerprint: 'a3', name: 'NodeDiskPressure', class: 'page', startsAt: minutesAgo(30), labels: { alertname: 'NodeDiskPressure' }, silencedBy: ['s-1'] },
    ],
  },
  '/api/v1/alerts/history': {
    items: [
      { id: 4, fingerprint: 'a1', name: 'BackupFailed', status: 'firing', startsAt: minutesAgo(360), observedAt: minutesAgo(359) },
      { id: 3, fingerprint: 'a9', name: 'ReleaseHeld', status: 'resolved', startsAt: minutesAgo(900), endsAt: minutesAgo(860), observedAt: minutesAgo(860) },
      { id: 2, fingerprint: 'a9', name: 'ReleaseHeld', status: 'firing', startsAt: minutesAgo(900), observedAt: minutesAgo(899) },
      { id: 1, fingerprint: 'a2', name: 'CertificateExpiring', status: 'firing', startsAt: minutesAgo(1500), observedAt: minutesAgo(1499) },
    ],
  },
}

const rollouts: Record<string, unknown[]> = {
  auth: [
    rollout(3807, '3.5.0', 5, [
      ['in_progress', 'auth 3.5.0: both members in analysis, 2 of 4', 2],
      ['in_progress', 'applied by Flux; auth waits for Flagger', 4],
      ['in_progress', 'auth 3.5.0 pinned', 5],
    ]),
    rollout(3790, '3.4.2', 4200, [
      ['success', 'auth 3.4.2 promoted', 4190],
      ['in_progress', 'auth 3.4.2 pinned', 4200],
    ]),
    rollout(3755, '3.4.1', 14000, [
      ['inactive', 'superseded by 3.4.2', 4190],
      ['success', 'auth 3.4.1 promoted', 13990],
    ]),
  ],
  delivery: [
    rollout(3686, '0.6.0', 820, [
      ['success', 'delivery 0.6.0 applied by Flux and ready', 819],
      ['in_progress', 'delivery 0.6.0 pinned', 820],
    ]),
  ],
  'estate-dashboard': [
    rollout(3870, '0.2.0', 120, [
      ['success', 'estate-dashboard 0.2.0 promoted', 117],
      ['in_progress', 'applied by Flux; estate-dashboard waits for Flagger', 118],
      ['in_progress', 'estate-dashboard 0.2.0 pinned', 120],
    ]),
  ],
  knowledge: [
    rollout(3601, '1.12.0', 2900, [
      ['failure', 'knowledge 1.12.0 did not become ready in 10m', 2880],
      ['in_progress', 'knowledge 1.12.0 pinned', 2900],
    ]),
  ],
  notes: [
    rollout(3812, '0.9.1', 62, [
      ['success', 'notes rolled back to 0.9.1 and paused', 61],
      ['in_progress', 'pre-rollback backup taken', 64],
    ]),
    rollout(3799, '0.10.0', 300, [['failure', 'notes 0.10.0 migration failed: relation "pages" already exists', 290]]),
  ],
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

/** Answers the API from the made-up estate. */
export function mockFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const request = input instanceof Request ? input : new Request(input, init)
  const { pathname } = new URL(request.url)
  const deploys = /^\/api\/v1\/estate\/projects\/([^/]+)\/deploys$/.exec(pathname)
  if (deploys) {
    const project = deploys[1] ?? ''
    const history = (rollouts[project] ?? []) as { version: string; at: string; commit: string }[]
    return Promise.resolve(json({ items: history.map((r) => ({ commit: r.commit, at: r.at, message: `chore(deploy): ${project} ${r.version}` })) }))
  }
  const ofProject = /^\/api\/v1\/estate\/projects\/([^/]+)\/rollouts$/.exec(pathname)
  if (ofProject) return Promise.resolve(json({ items: rollouts[ofProject[1] ?? ''] ?? [] }))
  if (pathname in answers) return Promise.resolve(json(answers[pathname]))
  return Promise.resolve(json({ type: 'about:blank', title: 'Not Found', status: 404 }, 404))
}
