import { useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import { listAlertHistoryOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { AlertEvent } from '../../../infrastructure/api/types.gen'
import { zProblem } from '../../../infrastructure/api/zod.gen'

/** What to tell the user about a failed request: the problem's detail when the server sent one. */
export function describeError(error: unknown): string {
  const problem = zProblem.safeParse(error)
  if (!problem.success) return 'Something went wrong. Try again.'
  return problem.data.detail ?? problem.data.title
}

/** Server state for the alert history. Components read events through this and never call the API. */
export function useAlertHistory() {
  const list = useQuery(listAlertHistoryOptions())

  return {
    events: computed<AlertEvent[]>(() => list.data.value?.items ?? []),
    isLoading: list.isPending,
    loadError: computed(() => (list.error.value ? describeError(list.error.value) : null)),
  }
}
