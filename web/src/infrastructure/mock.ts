/** A made-up estate with every state at once. Used only with VITE_MOCK=1. */

const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString()
const run = (id: number) => `https://github.com/JorisJonkers-dev/estate/actions/runs/${String(id)}`

const estate = ['auth', 'delivery', 'estate-dashboard', 'knowledge', 'notes']
const legacy = ['agents', 'data', 'grimoire', 'mail', 'media', 'tribelt']

const unit = (
  project: string,
  version: string,
  repository: string,
  ready = true,
  reason?: string,
  message?: string,
) => ({
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
      unit(
        'knowledge',
        '1.12.0',
        'JorisJonkers-dev/knowledge',
        false,
        'HealthCheckFailed',
        'Deployment knowledge-system/knowledge-api is not ready: 0 of 2 replicas',
      ),
      unit('notes', '0.9.1', 'JorisJonkers-dev/notes'),
    ],
  },
  '/api/v1/delivery/sources': {
    truncated: false,
    items: estate.map((p) => ({
      name: `project-${p}`,
      url: `oci://ghcr.io/jorisjonkers-dev/render/${p}`,
      digest: 'sha256:1',
      revision: 'sha256:1',
      ready: true,
      reason: 'Succeeded',
    })),
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
      {
        namespace: 'estate-dashboard-system',
        application: 'estate-dashboard',
        members: [{ process: 'estate-dashboard', phase: 'Succeeded', iterations: 4 }],
        serving: '0.2.0',
      },
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
      {
        number: 5,
        title: 'Dependency Dashboard',
        url: 'https://github.com/JorisJonkers-dev/estate/issues/5',
        labels: [],
        updatedAt: minutesAgo(30),
      },
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
      {
        fingerprint: 'a3',
        name: 'NodeDiskPressure',
        class: 'page',
        startsAt: minutesAgo(30),
        labels: { alertname: 'NodeDiskPressure' },
        silencedBy: ['s-1'],
      },
    ],
  },
  '/api/v1/alerts/history': {
    items: [
      {
        id: 4,
        fingerprint: 'a1',
        name: 'BackupFailed',
        status: 'firing',
        startsAt: minutesAgo(360),
        observedAt: minutesAgo(359),
      },
      {
        id: 3,
        fingerprint: 'a9',
        name: 'ReleaseHeld',
        status: 'resolved',
        startsAt: minutesAgo(900),
        endsAt: minutesAgo(860),
        observedAt: minutesAgo(860),
      },
      {
        id: 2,
        fingerprint: 'a9',
        name: 'ReleaseHeld',
        status: 'firing',
        startsAt: minutesAgo(900),
        observedAt: minutesAgo(899),
      },
      {
        id: 1,
        fingerprint: 'a2',
        name: 'CertificateExpiring',
        status: 'firing',
        startsAt: minutesAgo(1500),
        observedAt: minutesAgo(1499),
      },
    ],
  },
}

const rollouts: Record<string, unknown[]> = {
  'auth': [
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
  'delivery': [
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
  'knowledge': [
    rollout(3601, '1.12.0', 2900, [
      ['failure', 'knowledge 1.12.0 did not become ready in 10m', 2880],
      ['in_progress', 'knowledge 1.12.0 pinned', 2900],
    ]),
  ],
  'notes': [
    rollout(3812, '0.9.1', 62, [
      ['success', 'notes rolled back to 0.9.1 and paused', 61],
      ['in_progress', 'pre-rollback backup taken', 64],
    ]),
    rollout(3799, '0.10.0', 300, [['failure', 'notes 0.10.0 migration failed: relation "pages" already exists', 290]]),
  ],
}

// Each container with when it wrote its first and latest line, in minutes ago.
const stream = (pod: string, container: string, version: string, first: number, last: number) => ({
  pod,
  container,
  version,
  first: minutesAgo(first),
  last: minutesAgo(last),
})
const streams: Record<string, ReturnType<typeof stream>[]> = {
  notes: [
    stream('notes-migration-up-0-10-0-x7k2p', 'liquibase', '0.10.0', 302, 301),
    stream('notes-migration-down-0-10-0-9qv4m', 'liquibase-down', '0.10.0', 66, 65),
    stream('notes-6c8f7d9b5-q1w2e', 'notes', '0.10.0', 299, 290),
    stream('notes-primary-6d9f7c5b8-2hxlq', 'notes', '0.9.1', 60, 0.5),
    stream('notes-primary-6d9f7c5b8-8kd3s', 'notes', '0.9.1', 58, 1),
  ],
  delivery: [
    stream('collector-7d9c8b6f5-a1b2c', 'collector', '0.6.0', 790, 0.5),
    stream('reporter-5c6d7e8f9-d3e4f', 'reporter', '0.6.0', 790, 0.2),
    stream('release-gate-6b7c8d9e0-g5h6i', 'release-gate', '0.6.0', 790, 0.1),
    stream('release-gate-6b7c8d9e0-j7k8l', 'release-gate', '0.6.0', 785, 0.3),
    stream('flagger-8e9f0a1b2-m9n0p', 'flagger', '0.6.0', 790, 0.4),
  ],
  auth: [
    stream('auth-migration-up-3-5-0-p8d2c', 'liquibase', '3.5.0', 6, 5),
    stream('auth-api-7f8c9d6b5-kq2lm', 'auth-api', '3.5.0', 4, 0.2),
    stream('auth-ui-6b5c4d3e2-x9z8y', 'auth-ui', '3.5.0', 4, 0.3),
    stream('auth-migration-up-3-4-2-h3j4k', 'liquibase', '3.4.2', 4198, 4197),
    stream('auth-api-primary-5b6d7c8f9-z4w8n', 'auth-api', '3.4.2', 4190, 0.1),
    stream('auth-api-primary-5b6d7c8f9-r5t6u', 'auth-api', '3.4.2', 900, 0.4),
    stream('auth-api-primary-5b6d7c8f9-old1x', 'auth-api', '3.4.2', 4190, 905),
    stream('auth-ui-primary-7c9d8b6f5-m2p7q', 'auth-ui', '3.4.2', 4190, 2),
  ],
}

// The pods each Project runs: one per container that still writes, and each migration's Job.
const probes = { readiness: true, liveness: true, startup: false }
const pods = (project: string) =>
  (streams[project] ?? [])
    .filter((s) => s.pod.includes('migration') || Date.now() - Date.parse(s.last) < 15 * 60_000)
    .map((s) => {
      const job = s.pod.includes('migration')
      const looping = s.pod === 'flagger-8e9f0a1b2-m9n0p'
      return {
        name: s.pod,
        process: s.container,
        application: project,
        version: s.version,
        owner: job ? 'Job' : 'ReplicaSet',
        node: s.pod.length % 2 === 0 ? 'enschede-1' : 'frankfurt-contabo-1',
        phase: job ? 'Succeeded' : 'Running',
        started: s.first,
        ready: !job && !looping,
        deleting: false,
        containers: [
          {
            name: s.container,
            image: `ghcr.io/jorisjonkers-dev/${s.container}:${s.version}`,
            init: false,
            ready: !job && !looping,
            restarts: looping ? 14 : 0,
            probes: job ? { readiness: false, liveness: false, startup: false } : probes,
            ...(job
              ? { state: 'terminated', reason: 'Completed', since: s.last }
              : looping
                ? {
                    state: 'waiting',
                    reason: 'CrashLoopBackOff',
                    message: 'back-off 5m0s restarting failed container=flagger',
                    last: { reason: 'Error', exitCode: 1, at: minutesAgo(3) },
                  }
                : { state: 'running', since: s.first }),
          },
        ],
      }
    })

const check = (group: string, name: string, host: string, up: boolean, ms: number, error?: string) => ({
  key: `${group}_${name}`,
  name,
  group,
  host,
  at: minutesAgo(0.5),
  up,
  status: up ? 200 : 503,
  durationMs: ms,
  ...(error ? { error } : {}),
  conditions: [
    { condition: '[STATUS] == 200', success: up },
    { condition: '[RESPONSE_TIME] < 1500', success: ms < 1500 },
  ],
  recent: Array.from({ length: 20 }, (_, i) => up || i < 14),
})
const checks = {
  items: [
    check('public-apps', 'auth-api', 'auth-api.auth-system.svc.cluster.local', true, 41),
    check('public-apps', 'auth-ui', 'auth.jorisjonkers.dev', true, 188),
    check('public-apps', 'estate', 'estate-dashboard.estate-dashboard-system.svc.cluster.local', true, 12),
    check('public-apps', 'notes', 'notes.jorisjonkers.dev', false, 5000, 'context deadline exceeded'),
    check('internal-platform', 'release-gate', 'release-gate.delivery-system.svc.cluster.local', true, 6),
  ],
}

const logLines: Record<string, [number, string][]> = {
  'notes-migration-up-0-10-0-x7k2p/liquibase': [
    [302, 'Liquibase 4.29.2 by Liquibase'],
    [302, 'Running Changeset: db/changelog/0010-pages.yaml::0010-1::joris'],
    [301, 'ERROR: relation "pages" already exists'],
    [301, 'Migration failed for changeset db/changelog/0010-pages.yaml::0010-1::joris'],
    [301, 'Liquibase update failed: 1 changeset did not apply'],
  ],
  'notes-migration-down-0-10-0-9qv4m/liquibase-down': [
    [66, 'Rolling back to tag 0.9.1'],
    [66, 'Rolling back changeset db/changelog/0009-tags.yaml::0009-2::joris: nothing to undo'],
    [65, 'Rollback complete: database is at 0.9.1'],
  ],
}

function lines(pod: string, container: string) {
  const known = logLines[`${pod}/${container}`]
  if (known) return known.map(([m, text]) => ({ at: minutesAgo(m), text }))
  return Array.from({ length: 40 }, (_, i) => ({
    at: minutesAgo(40 - i),
    text: `{"time":"${minutesAgo(40 - i)}","level":"INFO","msg":"served","path":"/api/health","status":200,"ms":${String(3 + (i % 7))}}`,
  }))
}

const runs = {
  items: [
    {
      id: 38707081285,
      at: minutesAgo(5),
      trigger: 'auth 3.5.0 published',
      outcome: 'isolated',
      lock: '9c41e07b2a1d',
      toolkit: '0.14.0',
      snapshotAt: minutesAgo(125),
      moved: ['auth'],
      url: run(38707081285),
      fragments: [
        { project: 'auth', version: '3.5.0', outcome: 'moved', published: minutesAgo(6) },
        {
          project: 'knowledge',
          version: '1.13.1',
          outcome: 'isolated',
          reason: 'E_DUPLICATE_HOST: knowledge.jorisjonkers.dev is also declared by app',
          published: minutesAgo(2880),
        },
        {
          project: 'notes',
          version: '0.9.1',
          outcome: 'held',
          reason: 'paused after a rollback',
          published: minutesAgo(62),
        },
        { project: 'delivery', version: '0.6.0', outcome: 'unchanged', published: minutesAgo(820) },
        { project: 'estate-dashboard', version: '0.2.0', outcome: 'unchanged', published: minutesAgo(120) },
        {
          project: 'grimoire',
          version: '0.21.0',
          outcome: 'stale',
          reason: 'no fragment for 9 days',
          published: minutesAgo(12960),
        },
      ],
    },
    {
      id: 38706000001,
      at: minutesAgo(62),
      trigger: 'rollback of notes to 0.9.1',
      outcome: 'isolated',
      lock: '4b7d0e1f93c2',
      toolkit: '0.14.0',
      snapshotAt: minutesAgo(180),
      moved: ['notes'],
      url: run(38706000001),
      fragments: [{ project: 'notes', version: '0.9.1', outcome: 'moved', published: minutesAgo(400) }],
    },
    {
      id: 38700000002,
      at: minutesAgo(420),
      trigger: 'nightly',
      outcome: 'composed',
      lock: 'e2a85c4d10f7',
      toolkit: '0.14.0',
      snapshotAt: minutesAgo(500),
      moved: [],
      url: run(38700000002),
      fragments: [{ project: 'delivery', version: '0.6.0', outcome: 'unchanged', published: minutesAgo(820) }],
    },
  ],
}

const declarations: Record<string, unknown> = {
  auth: {
    version: '3.4.2',
    applications: [
      {
        id: 'auth',
        processes: [
          {
            name: 'auth-api',
            description: 'Signs users in and issues their tokens.',
            switchover: 'blue-green',
            replicas: 2,
          },
          { name: 'auth-ui', description: 'The sign-in pages.', switchover: 'blue-green', replicas: 1 },
        ],
        migration: { runner: 'liquibase', identity: 'auth-migration', provenAgainst: '3.4.2', undo: 'allowed' },
      },
    ],
    grants: [
      { process: 'auth-api', path: 'secret/data/platform/postgres/auth', access: 'read' },
      { process: 'auth-api', path: 'secret/data/auth-api', access: 'read' },
      { process: 'auth-api', path: 'transit/sign/auth-api-jwt', access: 'sign' },
    ],
    rollback: [
      'Back up auth_db on platform-postgres. Keep it 7 days.',
      'Wait for that backup to succeed, or stop',
      'Compose auth at 3.4.2, then pause it',
    ],
  },
  notes: {
    version: '0.9.1',
    applications: [
      {
        id: 'notes',
        processes: [{ name: 'notes', description: 'The notes wiki.', switchover: 'interrupted', replicas: 1 }],
        migration: {
          runner: 'liquibase',
          identity: 'notes-migration',
          provenAgainst: '0.9.1',
          undo: 'blocked',
          reason: '0.10.0 has a non-transactional changeset',
        },
      },
    ],
    grants: [{ process: 'notes', path: 'secret/data/platform/outline', access: 'read' }],
    rollback: ['Back up notes_db. Keep it 7 days.', 'Compose notes at 0.9.1, then pause it'],
  },
  delivery: {
    version: '0.6.0',
    applications: [
      {
        id: 'delivery',
        processes: [
          {
            name: 'collector',
            description: 'Saves the state of the cluster to the estate repository every 10 minutes.',
            switchover: 'continuous',
            replicas: 1,
          },
          {
            name: 'reporter',
            description: 'Turns Flux and Flagger events into GitHub deployment statuses.',
            switchover: 'continuous',
            replicas: 1,
          },
          {
            name: 'release-gate',
            description: 'Holds a new release until its checks pass, and runs the down migration when one is held.',
            switchover: 'continuous',
            replicas: 2,
          },
          {
            name: 'flagger',
            description: 'Switches traffic from the old version to the new one.',
            switchover: 'continuous',
            replicas: 1,
          },
        ],
      },
    ],
    grants: [{ process: 'collector', path: 'secret/data/delivery/github-app', access: 'read' }],
    rollback: [],
  },
}

const notes = (version: string) => ({
  version,
  body: `## [${version}](https://github.com/JorisJonkers-dev/x/compare/v0...v${version}) (2026-10-15)\n\n### Features\n\n* **api:** sign tokens with the transit key ([#97](https://github.com/JorisJonkers-dev/x/issues/97))\n* show each client's last sign-in\n\n### Bug Fixes\n\n* keep the session when the cookie domain changes`,
  url: 'https://github.com/JorisJonkers-dev/x/releases',
  publishedAt: minutesAgo(30),
})

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
    return Promise.resolve(
      json({
        items: history.map((r) => ({ commit: r.commit, at: r.at, message: `chore(deploy): ${project} ${r.version}` })),
      }),
    )
  }
  if (pathname === '/api/v1/estate/composition/runs') return Promise.resolve(json(runs))
  if (pathname === '/api/v1/estate/releases/notes')
    return Promise.resolve(json(notes(new URL(request.url).searchParams.get('version') ?? '')))
  const declared = /^\/api\/v1\/estate\/projects\/([^/]+)\/declaration$/.exec(pathname)
  if (declared) {
    const d = declarations[declared[1] ?? '']
    return Promise.resolve(
      d
        ? json(d)
        : json(
            {
              type: 'about:blank',
              title: 'Service Unavailable',
              status: 503,
              detail: 'Declarations are not read yet.',
            },
            503,
          ),
    )
  }
  const logStreams = /^\/api\/v1\/projects\/([^/]+)\/logs\/streams$/.exec(pathname)
  if (logStreams) return Promise.resolve(json({ items: streams[logStreams[1] ?? ''] ?? [] }))
  if (/^\/api\/v1\/projects\/[^/]+\/logs$/.test(pathname)) {
    const query = new URL(request.url).searchParams
    return Promise.resolve(json({ items: lines(query.get('pod') ?? '', query.get('container') ?? '') }))
  }
  const podsOf = /^\/api\/v1\/delivery\/projects\/([^/]+)\/pods$/.exec(pathname)
  if (podsOf) return Promise.resolve(json({ items: pods(podsOf[1] ?? ''), truncated: false }))
  if (pathname === '/api/v1/checks') return Promise.resolve(json(checks))
  const ofProject = /^\/api\/v1\/estate\/projects\/([^/]+)\/rollouts$/.exec(pathname)
  if (ofProject) return Promise.resolve(json({ items: rollouts[ofProject[1] ?? ''] ?? [] }))
  if (pathname in answers) return Promise.resolve(json(answers[pathname]))
  return Promise.resolve(json({ type: 'about:blank', title: 'Not Found', status: 404 }, 404))
}
