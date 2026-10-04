import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory } from 'vue-router'
import { configureApi } from '../infrastructure/http'
import App from './App.vue'
import { createAppRouter } from './router'

type Handler = (request: Request) => Response | Promise<Response>

function json(body: unknown, status = 200, contentType = 'application/json'): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': contentType } })
}

const fired = (name: string, id = 1) => ({
  id,
  fingerprint: '9f2c1a7b',
  name,
  status: 'firing',
  startsAt: '2026-10-03T07:00:00Z',
  observedAt: '2026-10-03T07:00:20Z',
})

const admin: Handler = () => json({ subject: 'user-1', name: 'joris' })
const nobody: Handler = () => json({ type: 'about:blank', title: 'Unauthorized', status: 401 }, 401, 'application/problem+json')

/** The paths the app asked the API for, in order. */
function asked(fetch: { mock: { calls: unknown[][] } }): string[] {
  return fetch.mock.calls.map(([input]) => new URL(input instanceof Request ? input.url : String(input)).pathname)
}

/** Mounts the whole app at path, with the network answered by handle and the session by session. */
async function mountApp(handle: Handler, path = '/', session: Handler = admin) {
  const fetch = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : new Request(input, init)
    return Promise.resolve((request.url.endsWith('/api/v1/session') ? session : handle)(request))
  })
  configureApi({ baseUrl: 'http://app.test', fetch })
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrapper = mount(App, { global: { plugins: [router, [VueQueryPlugin, { queryClient }]] } })
  await router.isReady()
  await flushPromises()
  await vi.dynamicImportSettled()
  await flushPromises()
  return { wrapper, fetch, router }
}

afterEach(() => {
  document.body.innerHTML = ''
})

describe('the alert history page', () => {
  it('lists the events the API returns, newest first as sent', async () => {
    const resolved = { ...fired('CollectorStopped', 2), status: 'resolved', endsAt: '2026-10-03T07:30:00Z', observedAt: '2026-10-03T07:30:20Z' }
    const { wrapper } = await mountApp(() => json({ items: [resolved, fired('CollectorStopped')] }))
    expect(wrapper.findAll('li').map((li) => li.find('.state').text())).toEqual(['Resolved after 30 min', 'Fired'])
    expect(wrapper.findAll('li .name').map((name) => name.text())).toEqual(['CollectorStopped', 'CollectorStopped'])
  })

  it('accepts a time with any offset, as RFC 3339 does', async () => {
    const { wrapper } = await mountApp(() => json({ items: [{ ...fired('ReleaseHeld'), observedAt: '2026-10-03T09:00:20+02:00' }] }))
    expect(wrapper.get('li time').attributes('datetime')).toBe('2026-10-03T09:00:20+02:00')
  })

  it('says so when nothing has been recorded', async () => {
    const { wrapper } = await mountApp(() => json({ items: [] }))
    expect(wrapper.text()).toContain('No alert has been recorded yet.')
  })

  it('shows the problem when the history cannot load', async () => {
    const { wrapper } = await mountApp(() =>
      json({ type: 'about:blank', title: 'Internal Server Error', status: 500 }, 500, 'application/problem+json'),
    )
    expect(wrapper.get('[role="alert"]').text()).toBe('Internal Server Error')
  })

  it('refuses a response the contract does not allow', async () => {
    const { wrapper } = await mountApp(() => json({ items: [{ ...fired('X'), status: 'pending', startsAt: 'yesterday' }] }))
    expect(wrapper.get('[role="alert"]').text()).toBe('Something went wrong. Try again.')
  })

  it('asks who is signed in, then for the history, and only reads', async () => {
    const { fetch } = await mountApp(() => json({ items: [] }))
    expect(asked(fetch)).toEqual(['/api/v1/session', '/api/v1/alerts/history'])
    for (const [request] of fetch.mock.calls) expect(request instanceof Request ? request.method : 'GET').toBe('GET')
  })

  it('sends unknown paths home', async () => {
    const { router } = await mountApp(() => json({ items: [] }), '/no/such/page')
    expect(router.currentRoute.value.path).toBe('/')
  })
})

describe('a browser nobody is signed in on', () => {
  it('is sent to sign in, and the page it asked for is never loaded', async () => {
    const { wrapper, fetch, router } = await mountApp(() => json({ items: [] }), '/', nobody)
    expect(router.currentRoute.value.fullPath).toBe('/sign-in')
    expect(wrapper.get('h1').text()).toBe('Sign in')
    expect(asked(fetch)).toEqual(['/api/v1/session'])
  })

  it('comes back to the page it asked for once signed in', async () => {
    const { wrapper, router } = await mountApp(() => json({ items: [] }), '/?since=yesterday', nobody)
    expect(router.currentRoute.value.query).toEqual({ next: '/?since=yesterday' })
    expect(wrapper.get('a.action').attributes('href')).toBe('/auth/login?next=%2F%3Fsince%3Dyesterday')
  })

  it('still sees the page when the API cannot say who is there, and the page reports it', async () => {
    const away: Handler = () => json({ type: 'about:blank', title: 'Service Unavailable', status: 503 }, 503, 'application/problem+json')
    const { wrapper, router } = await mountApp(away, '/', away)
    expect(router.currentRoute.value.fullPath).toBe('/')
    expect(wrapper.get('[role="alert"]').text()).toBe('Service Unavailable')
  })
})

describe('the sign-in page', () => {
  it('is the canvas page, outside the shell, and asks the API nothing', async () => {
    const { wrapper, fetch } = await mountApp(() => json({ items: [] }), '/sign-in', nobody)
    expect(wrapper.find('.shell').exists()).toBe(false)
    expect(wrapper.findAll('main')).toHaveLength(1)
    expect(wrapper.get('.name').text()).toBe('estate')
    expect(wrapper.get('h1').text()).toBe('Sign in')
    expect(wrapper.text()).toContain("The estate's delivery dashboard, for admins. You sign in with your jorisjonkers.dev account.")
    expect(wrapper.text()).toContain('You return here once auth has signed you in. Two-factor, if your account has it, is asked there.')
    expect(wrapper.findAll('.links a').map((a) => [a.text(), a.attributes('href')])).toEqual([
      ['Forgot your password?', 'https://auth.jorisjonkers.dev/forgot-password'],
      ['jorisjonkers.dev', 'https://jorisjonkers.dev'],
    ])
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(fetch).not.toHaveBeenCalled()
  })

  it('starts the sign-in on the server, returning home', async () => {
    const { wrapper } = await mountApp(() => json({ items: [] }), '/sign-in', nobody)
    const action = wrapper.get('a.action')
    expect(action.text()).toBe('Continue with jorisjonkers.dev')
    expect(action.attributes('href')).toBe('/auth/login')
  })

  it('returns to the page that was asked for, and to no other origin', async () => {
    const asked = await mountApp(() => json({ items: [] }), '/sign-in?next=%2Freleases%3Fpage%3D2', nobody)
    expect(asked.wrapper.get('a.action').attributes('href')).toBe('/auth/login?next=%2Freleases%3Fpage%3D2')
    const elsewhere = await mountApp(() => json({ items: [] }), '/sign-in?next=https%3A%2F%2Fevil.test', nobody)
    expect(elsewhere.wrapper.get('a.action').attributes('href')).toBe('/auth/login')
  })

  it('says so when a sign-in did not complete, without saying why', async () => {
    const { wrapper } = await mountApp(() => json({ items: [] }), '/sign-in?failed=1', nobody)
    expect(wrapper.get('[role="alert"]').text()).toBe('Signing in did not complete. Try again.')
  })
})

describe('the not-an-admin page', () => {
  it('is the canvas page, and asks the API nothing', async () => {
    const { wrapper, fetch } = await mountApp(() => json({ items: [] }), '/not-an-admin', nobody)
    expect(wrapper.find('.shell').exists()).toBe(false)
    expect(wrapper.get('main').classes()).toContain('wide')
    expect(wrapper.get('h1').text()).toBe('This dashboard is for admins')
    expect(wrapper.text()).toContain(
      'You are signed in, but your account does not hold the admin role. Ask an admin to grant it in auth, then sign in again.',
    )
    expect(wrapper.findAll('.links a').map((a) => [a.text(), a.attributes('href')])).toEqual([['Back to jorisjonkers.dev', 'https://jorisjonkers.dev']])
    expect(fetch).not.toHaveBeenCalled()
  })

  it('signs the account out of auth with a form post, so no link elsewhere can', async () => {
    const { wrapper } = await mountApp(() => json({ items: [] }), '/not-an-admin', nobody)
    const form = wrapper.get('form')
    expect([form.attributes('method'), form.attributes('action')]).toEqual(['post', '/auth/switch'])
    expect(form.get('button[type="submit"]').text()).toBe('Sign in as someone else')
  })
})
