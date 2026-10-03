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

/** Mounts the whole app at path, with the network answered by handle. */
async function mountApp(handle: Handler, path = '/') {
  const fetch = vi.fn((input: RequestInfo | URL, init?: RequestInit) =>
    Promise.resolve(handle(input instanceof Request ? input : new Request(input, init))),
  )
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

  it('asks the API once, and only reads', async () => {
    const { fetch } = await mountApp(() => json({ items: [] }))
    expect(fetch).toHaveBeenCalledTimes(1)
    const [request] = fetch.mock.calls[0] ?? []
    expect(request instanceof Request ? request.method : 'GET').toBe('GET')
  })

  it('sends unknown paths home', async () => {
    const { router } = await mountApp(() => json({ items: [] }), '/no/such/page')
    expect(router.currentRoute.value.path).toBe('/')
  })
})
