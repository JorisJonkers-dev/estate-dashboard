import { describe, expect, it } from 'vitest'
import { configureApi } from '../../../infrastructure/http'
import { whoIsSignedIn } from './useSession'

function answering(body: unknown, status: number, contentType = 'application/json') {
  configureApi({
    baseUrl: 'http://app.test',
    fetch: () => Promise.resolve(new Response(JSON.stringify(body), { status, headers: { 'Content-Type': contentType } })),
  })
}

const problem = (status: number, title: string) => ({ type: 'about:blank', title, status })

describe('whoIsSignedIn', () => {
  it('says yes when the API names the admin', async () => {
    answering({ subject: 'user-1', name: 'joris' }, 200)
    expect(await whoIsSignedIn()).toBe('yes')
  })

  it('says no only on a 401', async () => {
    answering(problem(401, 'Unauthorized'), 401, 'application/problem+json')
    expect(await whoIsSignedIn()).toBe('no')
  })

  it('does not know when the API fails', async () => {
    answering(problem(503, 'Service Unavailable'), 503, 'application/problem+json')
    expect(await whoIsSignedIn()).toBe('unknown')
  })

  it('does not know when the answer is not one the contract allows', async () => {
    answering({ subject: '' }, 200)
    expect(await whoIsSignedIn()).toBe('unknown')
  })

  it('does not know when the API cannot be reached', async () => {
    configureApi({ baseUrl: 'http://app.test', fetch: () => Promise.reject(new TypeError('Failed to fetch')) })
    expect(await whoIsSignedIn()).toBe('unknown')
  })
})
