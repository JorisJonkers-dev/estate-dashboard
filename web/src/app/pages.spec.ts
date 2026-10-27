import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory } from 'vue-router'
import { configureApi } from '../infrastructure/http'
import { mockFetch } from '../infrastructure/mock'
import App from './App.vue'
import { createAppRouter } from './router'

type Handler = (request: Request) => Promise<Response> | undefined

function json(body: unknown, status = 200): Promise<Response> {
  const type = status >= 400 ? 'application/problem+json' : 'application/json'
  return Promise.resolve(new Response(JSON.stringify(body), { status, headers: { 'Content-Type': type } }))
}

const unavailable = (detail: string) =>
  json({ type: 'about:blank', title: 'Service Unavailable', status: 503, detail }, 503)

/** Lets every read, and every read that waits on another, answer. */
async function settle() {
  for (let i = 0; i < 4; i++) {
    await flushPromises()
    await vi.dynamicImportSettled()
  }
}

/** Mounts the whole app at path over the made-up estate. `override` answers first where it wants to. */
async function visit(path: string, override: Handler = () => undefined) {
  const fetch = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : new Request(input, init)
    return override(request) ?? mockFetch(request)
  })
  configureApi({ baseUrl: 'http://app.test', fetch })
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrapper = mount(App, {
    attachTo: document.body,
    global: { plugins: [router, [VueQueryPlugin, { queryClient }]] },
  })
  await router.isReady()
  await settle()
  return { wrapper, fetch, router }
}

/** The button whose text is exactly the given text. */
function button(wrapper: VueWrapper, text: string) {
  const found = wrapper.findAll('button').find((b) => b.text() === text)
  if (!found)
    throw new Error(
      `no button "${text}" in ${wrapper
        .findAll('button')
        .map((b) => b.text())
        .join(', ')}`,
    )
  return found
}

const body = () => document.body.textContent

afterEach(() => {
  document.body.innerHTML = ''
})

describe('the overview', () => {
  it('shows the estate’s health, what needs you, and every project', async () => {
    const { wrapper } = await visit('/')
    const health = wrapper.get('dl[aria-label="Health"]').text()
    expect(health).toContain('Composition')
    expect(health).toContain('Delivery')
    expect(wrapper.get('#needs-h').text()).toBe('Needs you')
    expect(wrapper.findAll('tbody tr').length).toBeGreaterThan(5)
  })

  it('filters projects by state, and by name', async () => {
    const { wrapper } = await visit('/projects')
    const all = wrapper.findAll('tbody tr').length
    const live = wrapper.findAll('[role="radio"], button').find((b) => b.text().startsWith('Live'))
    await live?.trigger('click')
    expect(wrapper.findAll('tbody tr').length).toBeLessThan(all)
    await live?.trigger('click')
    expect(wrapper.findAll('tbody tr')).toHaveLength(all)

    await wrapper.get('input[type="search"]').setValue('aut')
    expect(wrapper.findAll('tbody th').map((t) => t.text())).toEqual(['auth'])
    await wrapper.get('input[type="search"]').setValue('nothing-like-this')
    expect(wrapper.text()).toContain('No projects match.')
  })

  it('says what it could not read', async () => {
    const { wrapper } = await visit('/', (r) =>
      new URL(r.url).pathname === '/api/v1/estate/handover' ? unavailable('GitHub is down.') : undefined,
    )
    expect(wrapper.get('[role="status"]').text()).toContain('GitHub is down.')
  })
})

describe('a project', () => {
  it('shows its version, and its health over the last hour above its actions', async () => {
    const { wrapper } = await visit('/projects/auth')
    expect(wrapper.get('h1').text()).toBe('auth3.4.2')
    const strip = wrapper.get('[role="toolbar"][aria-label^="Health of auth"]')
    expect(strip.findAll('button')).toHaveLength(120)
    await strip.findAll('button')[40]?.trigger('mouseenter')
    expect(wrapper.get('[role="tooltip"]').text()).toMatch(/ago · Degraded, a pod was not ready/)
    expect(wrapper.get('a[href="/projects/auth/rollout"]').text()).toBe('Latest rollout')
    expect(wrapper.find('a[aria-label="JorisJonkers-dev/auth-api on GitHub"]').exists()).toBe(true)
  })

  it('moves along its health strip with the arrow keys, one tab stop for the whole strip', async () => {
    const { wrapper } = await visit('/projects/auth')
    const blocks = wrapper.get('[role="toolbar"][aria-label^="Health of auth"]').findAll('button')
    expect(blocks.filter((b) => b.attributes('tabindex') === '0')).toHaveLength(1)
    await blocks[119]?.trigger('focus')
    await blocks[119]?.trigger('keydown', { key: 'ArrowLeft' })
    expect(wrapper.get('[role="tooltip"]').text()).toContain('Healthy')
    await blocks[118]?.trigger('keydown', { key: 'Home' })
    expect(wrapper.get('[role="tooltip"]').text()).toContain('No data')
    await blocks[0]?.trigger('mouseleave')
    expect(wrapper.find('[role="tooltip"]').exists()).toBe(false)
  })

  it('asks before it opens the pause or the rollback workflow', async () => {
    const { wrapper } = await visit('/projects/auth')
    await button(wrapper, 'Pause').trigger('click')
    await settle()
    expect(body()).toContain('Pause auth?')
    const open = [...document.querySelectorAll<HTMLAnchorElement>('a')].find((a) =>
      a.textContent.startsWith('Open the pause workflow'),
    )
    expect(open?.href).toBe('https://github.com/JorisJonkers-dev/estate/actions/workflows/pause.yml')
    ;[...document.querySelectorAll<HTMLButtonElement>('button')].find((b) => b.textContent === 'Cancel')?.click()
    await settle()
    expect(body()).not.toContain('Pause auth?')
    await button(wrapper, 'Roll back').trigger('click')
    await settle()
    expect(body()).toContain('Roll back auth?')
  })

  it('shows what runs now as one table, each container’s version, and the pods still on an older release', async () => {
    const { wrapper } = await visit('/projects/auth')
    const now = wrapper.get('table[aria-label="Running now"]').text()
    expect(now).toMatch(/auth-api\s*1 of 1( ready)?\s*auth-api\s*3\.5\.0/)
    const older = wrapper.get('table[aria-label="Still on release 3.4.2"]').text()
    expect(wrapper.text()).toContain('Still on release 3.4.2')
    expect(older).toMatch(/auth-api\s*2 of 2( ready)?\s*auth-api\s*3\.4\.2/)
  })

  it('shows each sidecar under its Process, and opens a container’s page from its row', async () => {
    const { wrapper, router } = await visit('/projects/notes')
    const rows = wrapper.get('table[aria-label="Running now"]').findAll('tbody tr')
    expect(rows.map((r) => r.get('th').text())).toEqual(['notes', 'file-backup sidecar'])
    expect(rows[1]?.text()).toContain('0.9.2')
    await rows[1]?.trigger('click')
    await settle()
    expect(router.currentRoute.value.fullPath).toBe('/containers/ghcr.io/jorisjonkers-dev/delivery/file-backup')
  })

  it('opens a container’s logs from its button, and a release’s changelog', async () => {
    const { wrapper } = await visit('/projects/auth')
    await button(wrapper, 'Logs of auth-api').trigger('click')
    await settle()
    expect(body()).toContain('auth · release 3.5.0 · auth-api 3.5.0')
    document.querySelector<HTMLButtonElement>('[aria-label="Close"]')?.click()
    await settle()
    await button(wrapper, 'Changelog').trigger('click')
    await settle()
    expect(body()).toContain('sign tokens with the transit key')
  })

  it('folds each release out of its row, the latest open, with what happened and its containers', async () => {
    const { wrapper } = await visit('/projects/auth')
    const folds = wrapper.findAll('#releases-h ~ ol > li button[aria-expanded]')
    expect(folds.map((f) => f.attributes('aria-expanded'))).toEqual(['true', 'false', 'false'])
    const latest = wrapper.get('#releases-h ~ ol > li').text()
    expect(latest).toContain('What happened')
    expect(latest).toContain('auth 3.5.0 pinned')
    expect(latest).toMatch(/Containers in this release\s*liquibase\s*3\.5\.0\s*auth-api\s*3\.5\.0/)
    await folds[0]?.trigger('click')
    await folds[1]?.trigger('click')
    expect(
      wrapper.findAll('#releases-h ~ ol > li button[aria-expanded]').map((f) => f.attributes('aria-expanded')),
    ).toEqual(['false', 'true', 'false'])
    expect(wrapper.find('a[href="/projects/auth/rollout?version=3.4.2"]').exists()).toBe(true)
  })

  it('shows its alerts, its status checks with their probes, and where its delivery stands', async () => {
    const { wrapper } = await visit('/projects/notes')
    const status = wrapper.get('#status-h').element.parentElement?.textContent ?? ''
    expect(status).toContain('notes.jorisjonkers.dev')
    expect(status).toMatch(/HTTP 503 · 5000 ms/)
    expect(wrapper.get('[aria-label="54 of the last 60 probes of notes succeeded"]').findAll('button')).toHaveLength(60)
    const delivery = wrapper.get('#delivery-h').element.parentElement?.textContent ?? ''
    expect(delivery).toMatch(/Pin\s*1 · release 0\.9\.1/)
    expect(delivery).toMatch(/Fetched\s*yes\s*Applied\s*applying, after estate-vso-secrets, apps-delivery/)
    expect(delivery).toContain('ghcr.io/jorisjonkers-dev/render/notes')
    expect(wrapper.text()).toContain('Resume')
    expect(wrapper.text()).toContain('Paused by joris')
  })

  it('says a gated Project switches through a canary', async () => {
    const { wrapper } = await visit('/projects/auth')
    expect(wrapper.get('#delivery-h').element.parentElement?.textContent).toContain('blue-green, through a canary')
  })

  it('tells a project on the old path, and one not in the estate, apart', async () => {
    expect((await visit('/projects/agents')).wrapper.text()).toContain('Still on the old path')
    document.body.innerHTML = ''
    expect((await visit('/projects/nowhere')).wrapper.text()).toContain("This project isn't in the estate.")
  })

  it('says when no container runs', async () => {
    const { wrapper } = await visit('/projects/knowledge')
    expect(wrapper.text()).toContain('No container of knowledge is running.')
  })

  it('marks a crash-looping Process', async () => {
    const { wrapper } = await visit('/projects/delivery')
    const flagger = wrapper
      .get('table[aria-label="Running now"]')
      .findAll('tbody tr')
      .find((r) => r.get('th').text() === 'flagger')
    expect(flagger?.text()).toContain('0 of 1')
    expect(flagger?.find('.text-crit').exists()).toBe(true)
  })

  it('blanks only the part whose source did not answer', async () => {
    const { wrapper } = await visit('/projects/auth', (r) =>
      /\/pods$|\/checks$|\/health\/projects\/auth$/.test(new URL(r.url).pathname) ? unavailable('down') : undefined,
    )
    expect(wrapper.get('#running-h').element.parentElement?.textContent).toContain(
      'Not read: the cluster did not answer.',
    )
    expect(wrapper.get('#status-h').element.parentElement?.textContent).toContain('Not read: Gatus did not answer.')
    expect(wrapper.text()).toContain('Health over the last hour: not read')
    expect(wrapper.find('#releases-h').exists()).toBe(true)
  })
})

describe('a rollout', () => {
  it('follows a release in flight live, its canary at the weight Flagger moved it to', async () => {
    const { wrapper } = await visit('/projects/auth/rollout')
    expect(wrapper.get('h1').text()).toMatch(/auth\s*3\.4\.2\s*→\s*3\.5\.0/)
    expect(wrapper.text()).toContain('Switching in')
    expect(wrapper.text()).toContain('Live')
    const lanes = wrapper.get('table[aria-label="Containers"]').findAll('tbody')
    expect(lanes[0]?.text()).toMatch(/auth-api.*Canary · 40%/)
    expect(lanes[0]?.get('[role="img"]').attributes('aria-label')).toBe('Traffic: new 40%, old 60%')
    expect(lanes[0]?.text()).toContain('canary round 2 · checks passing')
    expect(wrapper.text()).toContain('A canary takes more of the traffic each round')
  })

  it('draws the Release Gate as a barrier with what it checked, and says what a failure undoes', async () => {
    const { wrapper } = await visit('/projects/auth/rollout')
    const gate = wrapper.get('ul[aria-label="What the Release Gate checked"]').text()
    expect(gate).toContain('every primary runs 3.4.2, met')
    expect(gate).toContain('no non-transactional changeset, met')
    expect(wrapper.get('#undo-h').element.parentElement?.textContent).toContain(
      'The gate undoes the migration only when all of these hold:',
    )
  })

  it('replays a release from its start, playing and scrubbing through it', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const { wrapper } = await visit('/projects/auth/rollout')
    await wrapper.get('button[aria-label="Back to the start"]').trigger('click')
    expect(wrapper.text()).toContain('0:00 of')
    expect(wrapper.text()).not.toContain('Live')
    expect(button(wrapper, 'Back to live').exists()).toBe(true)
    expect(wrapper.get('table[aria-label="Containers"]').findAll('tbody')[0]?.text()).toContain('Waiting')
    await wrapper.get('button[aria-label="Play"]').trigger('click')
    vi.advanceTimersByTime(1000)
    await flushPromises()
    expect(wrapper.text()).toMatch(/0:(09|10|11) of/)
    await wrapper.get('button[aria-label="Pause"]').trigger('click')
    await wrapper.get('#scrub').setValue('75')
    expect(wrapper.text()).toContain('1:15 of')
    expect(wrapper.get('table[aria-label="Containers"]').findAll('tbody')[0]?.text()).toContain('Ready, no traffic')
    vi.useRealTimers()
  })

  it('jumps back to live from a replay, and follows a release in flight whenever it is opened', async () => {
    const { wrapper, router } = await visit('/projects/auth/rollout')
    await wrapper.get('button[aria-label="Back to the start"]').trigger('click')
    const clockText = () => wrapper.get('[aria-label="Replay"] time').text()
    await button(wrapper, 'Back to live').trigger('click')
    expect(wrapper.text()).toContain('Live')
    expect(clockText()).not.toMatch(/^0:00 of/)
    // A finished release opens at its end, and one in flight opened after it follows live, no Play pressed.
    await wrapper.get('button[aria-label="Back to the start"]').trigger('click')
    await router.push('/projects/auth/rollout?version=3.4.2')
    await settle()
    expect(clockText()).toMatch(/^(\d+):(\d+) of \1:\2$/)
    expect(wrapper.text()).not.toContain('Back to live')
    await router.push('/projects/auth/rollout')
    await settle()
    expect(wrapper.text()).toContain('Live')
    expect(wrapper.text()).not.toContain('Back to live')
  })

  it('rereads what drives a release every few seconds while one is in flight, and slowly once none is', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const reads = (fetch: ReturnType<typeof vi.fn>, path: string) =>
      fetch.mock.calls.filter(([input]) => String(input instanceof Request ? input.url : input).includes(path)).length
    for (const [project, more] of [
      ['auth', true],
      ['delivery', false],
    ] as const) {
      const { fetch, wrapper } = await visit(`/projects/${project}/rollout`)
      const before = ['/delivery/units', '/delivery/releases', '/rollouts', '/pods'].map((p) => reads(fetch, p))
      await vi.advanceTimersByTimeAsync(6_000)
      await settle()
      const after = ['/delivery/units', '/delivery/releases', '/rollouts', '/pods'].map((p) => reads(fetch, p))
      expect([project, after.every((n, i) => n > (before[i] ?? 0))]).toEqual([project, more])
      wrapper.unmount()
    }
    vi.useRealTimers()
  })

  it('shows a blue-green release Flagger rolled back as failed, its canary’s analysis and the old pod serving', async () => {
    // 2026-10-27: Flux applied the release and went Ready; the gate's checks failed, and Flagger rolled back.
    const { wrapper } = await visit('/projects/delivery/rollout', (r) =>
      new URL(r.url).pathname === '/api/v1/delivery/releases'
        ? json({
            truncated: false,
            items: [
              {
                namespace: 'delivery-system',
                application: 'delivery',
                members: [{ process: 'reporter', phase: 'Failed', iterations: 3, revision: 'sha256:new' }],
                unreadable: 'the Release Gate did not answer: connection refused',
              },
            ],
          })
        : undefined,
    )
    const header = wrapper.get('h1').element.parentElement?.textContent ?? ''
    expect(header).toContain('Failed')
    expect(header).not.toContain('Done')
    expect(wrapper.text()).toContain('blue-green, through a canary')
    const lanes = wrapper.get('table[aria-label="Containers"]').findAll('tbody')
    const reporter = lanes.find((l) => l.text().includes('reporter'))?.text() ?? ''
    expect(reporter).toContain('canary · rolled back after 3 iterations')
    expect(reporter).toContain('Flagger rolled it back after 3 iterations · the old pod keeps all traffic')
    expect(wrapper.text()).toContain('A blue-green canary runs its analysis while the old pod keeps all traffic')
  })

  it('shows a finished continuous release as done, its old pods stopped', async () => {
    const { wrapper } = await visit('/projects/delivery/rollout')
    expect(wrapper.text()).toContain('Done')
    expect(wrapper.text()).not.toContain('Live')
    const lane = wrapper.get('table[aria-label="Containers"]').findAll('tbody')[0]?.text() ?? ''
    expect(lane).toContain('Serving')
    expect(lane).toContain('Stopped')
    expect(wrapper.text()).toContain('A new pod shares traffic with the old one')
    expect(wrapper.text()).toContain('delivery switches continuously')
  })

  it('shows a failed release, the new pod failed and the old one still serving', async () => {
    const { wrapper } = await visit('/projects/notes/rollout?version=0.10.0')
    expect(wrapper.get('h1').text()).toMatch(/notes\s*0\.10\.0/)
    expect(wrapper.text()).toContain('Failed')
    const lane = wrapper.get('table[aria-label="Containers"]').findAll('tbody')[0]?.text() ?? ''
    expect(lane).toMatch(/Serving.*Failed/)
    expect(lane).toContain('never turned ready · the old pod keeps all traffic')
  })

  it('shows its sidecars and init containers under their Process, each with its own state and no health checks or traffic', async () => {
    const { wrapper } = await visit('/projects/notes/rollout')
    const rows = wrapper.get('table[aria-label="Containers"]').findAll('tbody')[0]?.findAll('tr') ?? []
    const [sidecar, init] = [rows[1], rows[2]]
    expect(sidecar?.text()).toMatch(/file-backup\s*sidecar/)
    expect(sidecar?.text()).toContain('Starting, not ready')
    expect(sidecar?.text()).toContain('takes no traffic')
    expect(sidecar?.get('[role="img"]').attributes('aria-label')).toBe('file-backup: Starting, not ready')
    expect(init?.text()).toMatch(/vault-agent-init\s*init, runs once/)
    expect(init?.text()).toContain('Done')
    expect(init?.find('[data-mark][data-outcome="succeeded"]').exists()).toBe(true)
    for (const row of [sidecar, init]) {
      expect(row?.find('[role="toolbar"]').exists()).toBe(false)
      expect(row?.find('[aria-label^="Traffic"]').exists()).toBe(false)
    }
  })

  it('shows each Job’s runs apart from the Processes: a span while it works, then how it ended', async () => {
    const { wrapper } = await visit('/projects/auth/rollout')
    const runs = wrapper.get('ul[aria-label="Runs"]').findAll('li')
    expect(runs.map((r) => r.text().split(/(?<=Job)/)[0])).toEqual([
      'vault-policy-2acd37c89500Job',
      'auth-migration-upJob',
      'postgres-data-backupCronJob',
    ])
    expect(runs[0]?.text()).toContain('Succeeded')
    expect(runs[0]?.text()).toContain('1 run · last 0:10–0:13, 3s')
    expect(runs[0]?.find('[data-mark][data-outcome="succeeded"]').exists()).toBe(true)
    expect(runs[0]?.find('[data-segment][data-state="running"]').classes()).not.toContain('motion-safe:animate-pulse')
    expect(wrapper.get('#runs-h').element.parentElement?.querySelector('[role="toolbar"]')).toBeNull()
  })

  it('replays a run as running while it works, and nothing before it starts', async () => {
    const { wrapper } = await visit('/projects/auth/rollout')
    await wrapper.get('#scrub').setValue('20')
    const runs = wrapper.get('ul[aria-label="Runs"]').findAll('li')
    expect(runs[1]?.text()).toContain('Running')
    expect(runs[1]?.text()).toContain('running since 0:15')
    expect(runs[1]?.get('[data-segment]').classes()).toContain('motion-safe:animate-pulse')
    expect(runs[1]?.find('[data-mark]').exists()).toBe(false)
    expect(runs[2]?.text()).toContain('Not run yet')
    expect(runs[2]?.find('[data-segment]').exists()).toBe(false)
  })

  it('shows a run that failed as failed and gone, not as an unhealthy pod', async () => {
    const { wrapper } = await visit('/projects/notes/rollout?version=0.10.0')
    const migration = wrapper.get('ul[aria-label="Runs"]').findAll('li')[1]
    expect(migration?.text()).toContain('notes-migration-up')
    expect(migration?.text()).toContain('Failed')
    expect(migration?.find('[data-mark][data-outcome="failed"]').exists()).toBe(true)
  })

  it('says when no Job ran in the window', async () => {
    const { wrapper } = await visit('/projects/auth/rollout', (r) =>
      new URL(r.url).pathname.endsWith('/rollout')
        ? Promise.resolve(
            new Response(JSON.stringify({ pods: [], weights: [], runs: [] }), {
              headers: { 'Content-Type': 'application/json' },
            }),
          )
        : undefined,
    )
    expect(wrapper.get('#runs-h').element.parentElement?.textContent).toContain(
      "No Job ran in auth's namespace in this release's window.",
    )
  })

  it('says when Prometheus cannot be read, and when there is no release', async () => {
    const { wrapper } = await visit('/projects/auth/rollout', (r) =>
      new URL(r.url).pathname.endsWith('/rollout') ? unavailable('down') : undefined,
    )
    expect(wrapper.get('#lanes-h').element.parentElement?.textContent).toContain('Not read: Prometheus did not answer.')
    expect(wrapper.get('#runs-h').element.parentElement?.textContent).toContain('Not read: Prometheus did not answer.')
    expect(wrapper.find('#order-h').exists()).toBe(true)
    document.body.innerHTML = ''
    expect((await visit('/projects/agents/rollout')).wrapper.text()).toContain('agents has no release to show yet.')
  })

  it('still shows its lanes when the Release Gate cannot be read, drawing no barrier', async () => {
    const { wrapper } = await visit('/projects/auth/rollout', (r) =>
      new URL(r.url).pathname === '/api/v1/delivery/releases' ? unavailable('down') : undefined,
    )
    expect(wrapper.find('table[aria-label="Containers"]').exists()).toBe(true)
    expect(wrapper.find('ul[aria-label="What the Release Gate checked"]').exists()).toBe(false)
    expect(wrapper.text()).toMatch(/Switches\s*continuously, no canary/)
  })

  it('is where the old Release in flight address leads', async () => {
    const { router } = await visit('/projects/auth/release')
    expect(router.currentRoute.value.fullPath).toBe('/projects/auth/rollout')
  })
})

describe('the alerts', () => {
  it('lists every alert by class and shows the one picked', async () => {
    const { wrapper } = await visit('/alerts')
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs.map((t) => t.text().split(' ')[0])).toContain('All')
    const items = wrapper.findAll('ul[aria-label="Alerts"] button')
    expect(items.length).toBeGreaterThan(1)
    await items.at(1)?.trigger('click')
    expect(items.at(1)?.attributes('aria-pressed')).toBe('true')
    for (const tab of tabs) await tab.trigger('click')
    expect(tabs.at(-1)?.attributes('aria-selected')).toBe('true')
  })

  it('opens the alert a link names', async () => {
    const { wrapper } = await visit('/alerts?alert=alert-a2')
    expect(wrapper.find('ul[aria-label="Alerts"] button[aria-pressed="true"]').exists()).toBe(true)
  })

  it('silences the picked alert, and says when Alertmanager refuses', async () => {
    let answer = json({ id: 's-9', fingerprint: 'a1', createdBy: 'joris', createdAt: '2026-10-15T10:00:00Z' }, 201)
    const silenced: string[] = []
    const { wrapper } = await visit('/alerts?alert=alert-a1', (r) => {
      if (r.method !== 'POST') return undefined
      silenced.push(new URL(r.url).pathname)
      return answer
    })
    await wrapper.get('select').setValue('4h')
    await button(wrapper, 'Silence').trigger('click')
    await settle()
    expect(silenced).toHaveLength(1)

    answer = unavailable('Alertmanager is down.')
    await button(wrapper, 'Silence').trigger('click')
    await settle()
    expect(wrapper.get('[role="alert"]').text()).toContain('Alertmanager is down.')
  })

  it('says when Alertmanager cannot be read', async () => {
    const { wrapper } = await visit('/alerts', (r) =>
      new URL(r.url).pathname === '/api/v1/alerts' ? unavailable('No Alertmanager.') : undefined,
    )
    expect(wrapper.text()).toContain('No Alertmanager.')
  })
})

describe('the composition runs', () => {
  it('lists each run and shows the fragments of the one picked', async () => {
    const { wrapper } = await visit('/composition')
    const runs = wrapper.findAll('ul[aria-label="Runs"] button')
    expect(runs.length).toBeGreaterThan(1)
    await runs.at(-1)?.trigger('click')
    expect(runs.at(-1)?.attributes('aria-pressed')).toBe('true')
  })

  it('shows a run’s lock by its first twelve digits, and a Project that published nothing without a release or an age', async () => {
    const at = new Date().toISOString()
    const { wrapper } = await visit('/composition', (r) =>
      new URL(r.url).pathname === '/api/v1/estate/composition/runs'
        ? json({
            items: [
              {
                id: 7,
                at,
                trigger: 'arr 1.2.0 released',
                outcome: 'isolated',
                lock: `sha256:422ebb5fc159${'e'.repeat(52)}`,
                moved: [],
                url: 'https://github.com/JorisJonkers-dev/estate/actions/runs/7',
                fragments: [{ project: 'media', outcome: 'stale', reason: 'E_PARTICIPANT_MISSING' }],
              },
            ],
          })
        : undefined,
    )
    expect(wrapper.text()).toContain('422ebb5fc159')
    expect(wrapper.text()).not.toContain('sha256:')
    const media = wrapper.findAll('tbody tr').find((tr) => tr.text().startsWith('media'))
    expect(media?.findAll('td').map((td) => td.text())).toEqual(['', 'StaleE_PARTICIPANT_MISSING', ''])
  })

  it('says when runs are not read, or there are none', async () => {
    const path = '/api/v1/estate/composition/runs'
    const down = await visit('/composition', (r) =>
      new URL(r.url).pathname === path ? unavailable('Not read yet.') : undefined,
    )
    expect(down.wrapper.text()).toContain('Not read yet.')
    document.body.innerHTML = ''
    const none = await visit('/composition', (r) =>
      new URL(r.url).pathname === path ? json({ items: [] }) : undefined,
    )
    expect(none.wrapper.text()).toContain('No runs yet')
  })
})

describe('the drawer when a read fails or holds nothing', () => {
  const failing = (pattern: RegExp, answer: () => Promise<Response>) => (r: Request) =>
    pattern.test(new URL(r.url).pathname) ? answer() : undefined

  it('says when a container wrote nothing, or its logs cannot be read', async () => {
    const empty = await visit(
      '/projects/delivery',
      failing(/\/logs$/, () => json({ items: [] })),
    )
    await button(empty.wrapper, 'Logs of collector').trigger('click')
    await settle()
    expect(body()).toContain('Nothing in the last 7 days.')
    document.body.innerHTML = ''

    const down = await visit(
      '/projects/delivery',
      failing(/\/logs$/, () => unavailable('Loki is down.')),
    )
    await button(down.wrapper, 'Logs of collector').trigger('click')
    await settle()
    expect(body()).toContain('Loki is down.')
  })

  it('says when a container wrote nothing in the window, or the streams cannot be read', async () => {
    const quiet = await visit(
      '/projects/notes',
      failing(/\/logs\/streams$/, () => json({ items: [] })),
    )
    await button(quiet.wrapper, 'Logs of file-backup').trigger('click')
    await settle()
    expect(body()).toContain('file-backup wrote nothing in the last hours.')
    document.body.innerHTML = ''
    const down = await visit(
      '/projects/notes',
      failing(/\/logs\/streams$/, () => unavailable('down')),
    )
    await button(down.wrapper, 'Logs of notes').trigger('click')
    await settle()
    expect(body()).toContain('Loki did not answer.')
  })

  it('names each canary of a gated Process by its attempt, and says which has stopped, when no primary is listed', async () => {
    const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString()
    // The Release Gate says which Processes Flagger switches; the declaration is not read yet.
    const { wrapper } = await visit('/projects/auth', (r) => {
      const path = new URL(r.url).pathname
      if (path.endsWith('/declaration')) return unavailable('Declarations are not read yet.')
      if (!path.endsWith('/logs/streams')) return undefined
      return json({
        items: [
          { pod: 'auth-api-5b6c7d8e9-t8f8w', container: 'auth-api', first: minutesAgo(90), last: minutesAgo(60) },
          { pod: 'auth-api-5b6c7d8e9-zpmh6', container: 'auth-api', first: minutesAgo(30), last: minutesAgo(1) },
        ],
      })
    })
    await button(wrapper, 'Logs of auth-api').trigger('click')
    await settle()
    const pods = [...document.querySelectorAll('[aria-label="Pod"] button')].map((b) => b.textContent.trim())
    expect(pods).toEqual([
      expect.stringMatching(/^canary · attempt 2\s*, running$/),
      expect.stringMatching(/^canary · attempt 1\s*stopped$/),
    ])
  })

  it('names the pods of a Process the Release Gate does not switch by their suffix', async () => {
    const { wrapper } = await visit('/projects/delivery', (r) => {
      const path = new URL(r.url).pathname
      if (path.endsWith('/delivery/releases'))
        return json({
          items: [
            { namespace: 'delivery-system', application: 'delivery', members: [{ process: 'api', iterations: 0 }] },
            // Another Project's member of the same name gates only that Project's pods.
            { namespace: 'notes-system', application: 'notes', members: [{ process: 'collector', iterations: 0 }] },
          ],
          truncated: false,
        })
      if (!path.endsWith('/logs/streams')) return undefined
      return json({
        items: [
          { pod: 'collector-6f8d9c7b5-7k2lp', container: 'collector', first: new Date().toISOString() },
          { pod: 'collector-6f8d9c7b5-q9x4d', container: 'collector', first: new Date(0).toISOString() },
        ],
      })
    })
    await button(wrapper, 'Logs of collector').trigger('click')
    await settle()
    const pods = [...document.querySelectorAll('[aria-label="Pod"] button')].map((b) => b.textContent.trim())
    expect(pods.map((p) => p.split(/\s/)[0])).toEqual(['7k2lp', 'q9x4d'])
  })

  it('says when a release has no changelog', async () => {
    const { wrapper } = await visit(
      '/projects/auth',
      failing(/\/releases\/notes$/, () =>
        json({ type: 'about:blank', title: 'Not Found', status: 404, detail: 'This release has no changelog.' }, 404),
      ),
    )
    await button(wrapper, 'Changelog').trigger('click')
    await settle()
    expect(body()).toContain('This release has no changelog.')
  })
})

describe('the containers', () => {
  const names = (wrapper: VueWrapper) => wrapper.findAll('tbody th').map((t) => t.text())

  it('lists each image once with its version and at most one mark, and where it runs', async () => {
    const { wrapper } = await visit('/containers')
    expect(names(wrapper)).toEqual(['auth-api', 'file-backup', 'jellyseerr', 'n8n', 'pgvector', 'traefik', 'vault'])
    const row = (name: string) =>
      wrapper
        .findAll('tbody tr')
        .find((r) => r.get('th').text() === name)
        ?.text() ?? ''
    expect(row('n8n')).toContain('2.43.8')
    expect(row('n8n')).toContain('2.44.0 ready')
    expect(row('jellyseerr')).toContain('drift')
    expect(row('vault')).toContain('3 versions')
    expect(row('traefik')).toContain('from tag')
    expect(row('pgvector')).toMatch(/pg17\s*no full version/)
    expect(row('auth-api')).toContain('2 versions')
  })

  it('counts and applies each filter, and searches by image, Project or Process', async () => {
    const { wrapper } = await visit('/containers')
    expect(button(wrapper, 'Platform 3').exists()).toBe(true)
    await button(wrapper, 'Needs update 1').trigger('click')
    expect(names(wrapper)).toEqual(['n8n'])
    await button(wrapper, 'All 7').trigger('click')
    await wrapper.get('input[type="search"]').setValue('vault-agent')
    expect(names(wrapper)).toEqual(['vault'])
    await wrapper.get('input[type="search"]').setValue('nothing-like-this')
    expect(wrapper.text()).toContain('No container matches.')
    await button(wrapper, 'Show all').trigger('click')
    expect(names(wrapper)).toHaveLength(7)
  })

  it('sorts a column up, down, then back to the order it came in', async () => {
    const { wrapper } = await visit('/containers')
    const version = () => wrapper.findAll('th[aria-sort]')[1]
    await version()?.get('button').trigger('click')
    expect(version()?.attributes('aria-sort')).toBe('ascending')
    expect(names(wrapper)[0]).toBe('file-backup')
    await version()?.get('button').trigger('click')
    expect(version()?.attributes('aria-sort')).toBe('descending')
    expect(names(wrapper)[0]).toBe('pgvector')
    await version()?.get('button').trigger('click')
    expect(version()?.attributes('aria-sort')).toBe('none')
    expect(names(wrapper)[0]).toBe('auth-api')
  })

  it('opens an image’s page from anywhere on its row, and its logs from the row’s button', async () => {
    const { wrapper, router } = await visit('/containers')
    const n8n = wrapper.findAll('tbody tr').find((r) => r.get('th').text() === 'n8n')
    await n8n?.trigger('click')
    await settle()
    expect(router.currentRoute.value.fullPath).toBe('/containers/ghcr.io/jorisjonkers-dev/n8n')
    document.body.innerHTML = ''
    const again = await visit('/containers')
    await button(again.wrapper, 'Logs of auth-api').trigger('click')
    await settle()
    expect(body()).toContain('auth · 3.5.0')
  })

  it('says when the cluster cannot be read', async () => {
    const { wrapper } = await visit('/containers', (r) =>
      new URL(r.url).pathname === '/api/v1/delivery/containers' ? unavailable('down') : undefined,
    )
    expect(wrapper.text()).toContain('Not read: the cluster did not answer.')
  })
})

describe('a container', () => {
  it('shows its facts, its pods, its history and the newer version waiting', async () => {
    const { wrapper } = await visit('/containers/ghcr.io/jorisjonkers-dev/n8n')
    expect(wrapper.get('h1').text()).toBe('n8n 2.43.8')
    expect(wrapper.text()).toContain('2.44.0 ready')
    expect(wrapper.text()).toMatch(/Version from\s*the images lock/)
    expect(wrapper.text()).toContain('matches the lock')
    expect(wrapper.get('table[aria-label="Pods"]').text()).toContain('enschede-t1000-1')
    expect(wrapper.findAll('#history-h ~ ol li').map((li) => li.text())).toEqual([
      expect.stringMatching(/2\.43\.8\s*from 2\.43\.6 · shipped in n8n 2\.43\.10/),
      expect.stringContaining('2.43.6'),
      expect.stringMatching(/2\.42\.1\s*first recorded/),
    ])
    expect(wrapper.get('#newer-h').element.parentElement?.textContent).toContain('2.44.0 is ready to merge.')
  })

  it('shows which Processes run which version, and that fleet-infra deploys it', async () => {
    const { wrapper } = await visit('/containers/docker.io/hashicorp/vault')
    const spread = wrapper.get('#spread-h').element.parentElement?.textContent ?? ''
    expect(spread).toMatch(/2\.0\.4\s*vault-agent\s*newest/)
    expect(spread).toMatch(/1\.21\.2\s*vault/)
    expect(wrapper.text()).toMatch(/Deployed by\s*fleet-infra/)
    expect(wrapper.text()).toContain('not pinned by digest')
    expect(wrapper.text()).toContain('Renovate watches this image in fleet-infra.')
    expect(wrapper.text()).toContain('No version change recorded yet.')
  })

  it('shows a drifting container with both digests', async () => {
    const { wrapper } = await visit('/containers/ghcr.io/jorisjonkers-dev/jellyseerr')
    expect(wrapper.text()).toMatch(/0c41d9e2a7b3\s*runs, but its pod pins 5ab0c37e19f2/)
  })

  it('links the repository that builds an image of the estate from the Project that runs it, and none where units are not read', async () => {
    const { wrapper } = await visit('/containers/ghcr.io/jorisjonkers-dev/auth-api')
    expect(wrapper.find('a[aria-label="JorisJonkers-dev/auth-api on GitHub"]').exists()).toBe(true)
    document.body.innerHTML = ''
    const blind = await visit('/containers/ghcr.io/jorisjonkers-dev/auth-api', (r) =>
      new URL(r.url).pathname === '/api/v1/delivery/units' ? unavailable('down') : undefined,
    )
    expect(blind.wrapper.find('a[aria-label$="on GitHub"]').exists()).toBe(false)
  })

  it('says an image whose tag names no full version has none', async () => {
    const { wrapper } = await visit('/containers/docker.io/pgvector/pgvector')
    expect(wrapper.text()).toContain('nowhere: its tag names no full version')
  })

  it('opens a pod’s project from its row, and its logs from its button', async () => {
    const { wrapper, router } = await visit('/containers/ghcr.io/jorisjonkers-dev/delivery/file-backup')
    expect(wrapper.find('a[aria-label="JorisJonkers-dev/delivery on GitHub"]').exists()).toBe(true)
    expect(wrapper.get('table[aria-label="Pods"]').text()).toContain('file-backup, beside notes')
    await button(wrapper, 'Logs of notes-primary-6d9f7c5b8-2hxlq').trigger('click')
    await settle()
    expect(body()).toContain('pod notes-primary-6d9f7c5b8-2hxlq')
    document.querySelector<HTMLButtonElement>('[aria-label="Close"]')?.click()
    await settle()
    await wrapper.get('table[aria-label="Pods"] tbody tr').trigger('click')
    await settle()
    expect(router.currentRoute.value.fullPath).toBe('/projects/notes')
  })

  it('blanks only what a source that does not answer would say', async () => {
    const { wrapper } = await visit('/containers/ghcr.io/jorisjonkers-dev/n8n', (r) =>
      /\/updates$|\/history$/.test(new URL(r.url).pathname) ? unavailable('down') : undefined,
    )
    expect(wrapper.get('#newer-h').element.parentElement?.textContent).toContain('Not read: GitHub did not answer.')
    expect(wrapper.get('#history-h').element.parentElement?.textContent).toContain(
      'Not read: the cluster did not answer.',
    )
    expect(wrapper.find('table[aria-label="Pods"]').exists()).toBe(true)
  })

  it('says when the estate runs no such image, or the cluster cannot be read', async () => {
    expect((await visit('/containers/ghcr.io/nobody/none')).wrapper.text()).toContain(
      'The estate runs no image ghcr.io/nobody/none.',
    )
    document.body.innerHTML = ''
    const { wrapper } = await visit('/containers/ghcr.io/jorisjonkers-dev/n8n', (r) =>
      new URL(r.url).pathname === '/api/v1/delivery/containers' ? unavailable('down') : undefined,
    )
    expect(wrapper.text()).toContain('Not read: the cluster did not answer.')
  })
})
