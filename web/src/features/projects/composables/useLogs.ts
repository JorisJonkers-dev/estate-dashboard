import { useQuery } from '@tanstack/vue-query'
import { computed, ref, watch, type Ref } from 'vue'
import { listLogLinesOptions, listLogStreamsOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { Stream } from '../../../infrastructure/api/types.gen'
import { describeError } from '../../alerts/composables/useAlertHistory'
import { groupStreams, streamKey } from '../domain/logs'

/** A Project's containers that wrote logs, and the lines of the one picked, from Loki. */
export function useLogs(project: Ref<string>) {
  const hours = ref(24)
  const picked = ref<Stream | null>(null)

  const streams = useQuery(computed(() => ({ ...listLogStreamsOptions({ path: { project: project.value }, query: { hours: hours.value } }), refetchInterval: 60_000 })))
  const groups = computed(() => groupStreams(streams.data.value?.items ?? []))

  // Pick the first container, and drop a pick that has gone.
  watch(
    () => streams.data.value?.items,
    (items) => {
      const all = items ?? []
      if (picked.value && all.some((s) => streamKey(s) === streamKey(picked.value as Stream))) return
      picked.value = groups.value[0]?.streams[0] ?? null
    },
  )

  const lines = useQuery(
    computed(() => ({
      ...listLogLinesOptions({
        path: { project: project.value },
        query: { pod: picked.value?.pod ?? '', container: picked.value?.container ?? '', hours: hours.value, limit: 1000 },
      }),
      enabled: picked.value !== null,
      refetchInterval: 10_000,
    })),
  )

  return {
    hours,
    picked,
    groups,
    lines: computed(() => lines.data.value?.items ?? []),
    isLoading: computed(() => streams.isPending.value),
    linesLoading: computed(() => picked.value !== null && lines.isPending.value),
    unread: computed(() => (streams.error.value ? describeError(streams.error.value) : lines.error.value ? describeError(lines.error.value) : null)),
  }
}
