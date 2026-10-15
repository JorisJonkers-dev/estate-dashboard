import { describe, expect, it, vi } from 'vitest'
import { createMemoryHistory } from 'vue-router'
import { backToSignIn } from './expired'
import { createAppRouter } from './router'

const unauthorized = { type: 'about:blank', title: 'Unauthorized', status: 401 }

async function at(path: string) {
  const router = createAppRouter(createMemoryHistory(), () => Promise.resolve('yes'))
  await router.push(path)
  return router
}

describe('a read refused mid-visit', () => {
  it('sends the browser to sign in, to come back to the page it was on', async () => {
    const router = await at('/alerts?x=1')
    backToSignIn(router)(unauthorized)
    await vi.waitFor(() => {
      expect(router.currentRoute.value.name).toBe('sign-in')
    })
    expect(router.currentRoute.value.query).toEqual({ next: '/alerts?x=1' })
  })

  it('from home, asks for no return page', async () => {
    const router = await at('/')
    backToSignIn(router)(unauthorized)
    await vi.waitFor(() => {
      expect(router.currentRoute.value.name).toBe('sign-in')
    })
    expect(router.currentRoute.value.query).toEqual({})
  })

  it('leaves any other failure, and a public page, where they are', async () => {
    const router = await at('/alerts')
    backToSignIn(router)({ ...unauthorized, status: 503 })
    backToSignIn(router)(new Error('offline'))
    await new Promise((resolve) => setTimeout(resolve))
    expect(router.currentRoute.value.name).toBe('alerts')
    const signIn = await at('/sign-in')
    backToSignIn(signIn)(unauthorized)
    await new Promise((resolve) => setTimeout(resolve))
    expect(signIn.currentRoute.value.query).toEqual({})
  })
})
