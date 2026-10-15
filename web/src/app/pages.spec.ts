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
  it('shows what runs now, what finished, its releases, and its alerts', async () => {
    const { wrapper } = await visit('/projects/auth')
    const running = wrapper.get('#running-h').element.parentElement?.textContent ?? ''
    expect(running).toContain('3.4.2')
    expect(running).toMatch(/auth-api\s*2 running/)
    expect(running).toContain('Signs users in and issues their tokens.')
    expect(running).toMatch(/liquibase\s*finished/)
    expect(wrapper.findAll('#releases-h ~ ol > li')).toHaveLength(3)
    expect(wrapper.get('#alerts-h').element.parentElement?.parentElement?.textContent).toContain(
      'auth switching to 3.5.0',
    )
    expect(wrapper.text()).toContain('Release in flight')
  })

  it('opens a release’s details, a container’s logs, and its changelog in the drawer', async () => {
    const { wrapper } = await visit('/projects/auth')
    await button(wrapper, 'Details').trigger('click')
    expect(wrapper.text()).toContain('Built from')
    await button(wrapper, 'Hide').trigger('click')
    expect(wrapper.text()).not.toContain('Built from')

    await button(wrapper, 'Logs').trigger('click')
    await settle()
    expect(body()).toContain('auth · 3.4.2')
    const pods = [...document.querySelectorAll<HTMLButtonElement>('[aria-label="Pod"] button')]
    expect(pods.map((p) => p.textContent)).toEqual(expect.arrayContaining([expect.stringContaining('primary')]))
    pods.at(-1)?.click()
    await settle()
    expect(pods.at(-1)?.getAttribute('aria-pressed')).toBe('true')

    document.querySelector<HTMLButtonElement>('[aria-label="Close"]')?.click()
    await settle()
    await button(wrapper, 'Changelog').trigger('click')
    await settle()
    expect(body()).toContain('sign tokens with the transit key')
  })

  it('shows a paused project’s pause, and its earlier releases’ containers', async () => {
    const { wrapper } = await visit('/projects/notes')
    expect(wrapper.text()).toContain('Resume')
    expect(wrapper.text()).toContain('Paused')
    const details = wrapper.findAll('button').filter((b) => b.text() === 'Details')
    await details.at(-1)?.trigger('click')
    expect(wrapper.text()).toContain('migration')
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
})

describe('a release switching in', () => {
  it('shows its order, its barrier and when it would undo', async () => {
    const { wrapper } = await visit('/projects/auth/release')
    expect(wrapper.get('#order-h').text()).toBe('Order')
    expect(wrapper.text()).toContain('Migration up')
    expect(wrapper.text()).toContain('auth-api')
    expect(wrapper.text()).toContain('The down migration runs only when all of these hold:')
  })

  it('says when nothing switches', async () => {
    const { wrapper } = await visit('/projects/delivery/release')
    expect(wrapper.text()).toContain('Nothing switching')
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

describe('a release in each state the Release Gate reports', () => {
  const gate = (release: Record<string, unknown>) => (r: Request) =>
    new URL(r.url).pathname === '/api/v1/delivery/releases'
      ? json({ items: [{ namespace: 'auth-system', application: 'auth', ...release }], truncated: false })
      : undefined

  it('shows a migration still running, a member not seen yet and a held down migration', async () => {
    const { wrapper } = await visit(
      '/projects/auth/release',
      gate({
        members: [
          { process: 'auth-api', phase: 'Initializing', iterations: 0 },
          { process: 'auth-worker', iterations: 0 },
        ],
        migration: { identity: 'auth-migration', nonTransactional: true },
      }),
    )
    expect(wrapper.text()).toContain('Running')
    expect(wrapper.text()).toContain('not seen yet')
    expect(wrapper.text()).toContain('every primary runs the tested release')
    expect(wrapper.text()).toContain('Waiting')
  })

  it('shows a release without a migration whose members all passed their checks, one failed', async () => {
    const { wrapper } = await visit(
      '/projects/auth/release',
      gate({
        members: [
          { process: 'auth-api', phase: 'Succeeded', iterations: 4 },
          { process: 'auth-ui', phase: 'Failed', iterations: 4 },
        ],
        serving: '3.4.2',
        pinned: '3.5.0',
        since: new Date(Date.now() - 60_000).toISOString(),
      }),
    )
    expect(wrapper.text()).toContain('Skipped')
    expect(wrapper.text()).toContain('May promote')
    expect(wrapper.text()).toContain('Started')
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
    await button(empty.wrapper, 'Logs').trigger('click')
    await settle()
    expect(body()).toContain('Nothing in the last 7 days.')
    document.body.innerHTML = ''

    const down = await visit(
      '/projects/delivery',
      failing(/\/logs$/, () => unavailable('Loki is down.')),
    )
    await button(down.wrapper, 'Logs').trigger('click')
    await settle()
    expect(body()).toContain('Loki is down.')
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
