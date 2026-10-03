import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'
import { configureApi } from '../../../infrastructure/http'
import { describeError, useAlertHistory } from './useAlertHistory'

describe('useAlertHistory', () => {
  it('has no events before the first response', () => {
    // A request that never settles keeps the query pending for the whole test.
    configureApi({ baseUrl: 'http://app.test', fetch: () => new Promise<Response>(() => undefined) })
    let history: ReturnType<typeof useAlertHistory> | undefined
    mount(defineComponent({ setup: () => ((history = useAlertHistory()), () => h('div')) }), {
      global: { plugins: [[VueQueryPlugin, { queryClient: new QueryClient() }]] },
    })
    expect(history?.events.value).toEqual([])
    expect(history?.isLoading.value).toBe(true)
    expect(history?.loadError.value).toBeNull()
  })
})

describe('describeError', () => {
  it("prefers a problem's detail, then its title", () => {
    expect(describeError({ type: 'about:blank', title: 'Bad Request', status: 400, detail: 'Fix it.' })).toBe('Fix it.')
    expect(describeError({ type: 'about:blank', title: 'Bad Request', status: 400 })).toBe('Bad Request')
  })

  it('falls back for anything that is not a problem', () => {
    expect(describeError(new TypeError('Failed to fetch'))).toBe('Something went wrong. Try again.')
  })
})
