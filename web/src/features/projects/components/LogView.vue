<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { computed, nextTick, ref, useTemplateRef, watch } from 'vue'
import { listLogLinesOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { Stream } from '../../../infrastructure/api/types.gen'
import { describeError } from '../../alerts/composables/useAlertHistory'
import { readLine, window } from '../domain/logs'

/** The lines of one Process's containers: one pod at a time, newest at the bottom. */
const props = defineProps<{ project: string; streams: Stream[] }>()
const pod = ref(props.streams[0]?.pod ?? '')
const stream = computed(() => props.streams.find((s) => s.pod === pod.value) ?? props.streams[0])

const lines = useQuery(
  computed(() => ({
    ...listLogLinesOptions({ path: { project: props.project }, query: { pod: stream.value?.pod ?? '', container: stream.value?.container ?? '', hours: window, limit: 1000 } }),
    enabled: stream.value !== undefined,
    refetchInterval: 10_000,
  })),
)
const read = computed(() => (lines.data.value?.items ?? []).map((l) => ({ at: l.at, ...readLine(l.text) })))
const failure = computed(() => (lines.error.value ? describeError(lines.error.value) : null))

const time = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' })
const level: Record<string, string> = { error: 'text-crit', warn: 'text-warn', warning: 'text-warn', info: 'text-dim', debug: 'text-dim' }

// Keep the newest line in view as lines arrive.
const view = useTemplateRef<HTMLElement>('view')
watch(read, async () => {
  await nextTick()
  if (view.value) view.value.scrollTop = view.value.scrollHeight
})
</script>

<template>
  <div class="flex h-full flex-col">
    <div v-if="streams.length > 1" role="group" aria-label="Pod" class="flex flex-wrap gap-2 border-b border-line px-6 py-3">
      <button
        v-for="s in streams"
        :key="s.pod"
        type="button"
        :aria-pressed="s.pod === pod"
        class="cursor-pointer rounded-full border border-control bg-transparent px-3 py-1 font-mono text-xs text-nav transition-colors hover:text-white aria-pressed:border-accent aria-pressed:text-white"
        @click="pod = s.pod"
      >
        {{ s.pod }}
      </button>
    </div>
    <div ref="view" tabindex="0" aria-label="Log lines" class="min-h-0 grow overflow-auto bg-[#181818] px-6 py-4 font-mono text-[13px] leading-relaxed">
      <p v-if="failure" class="m-0 text-warn">{{ failure }}</p>
      <p v-else-if="lines.isPending.value" class="m-0 text-text-muted">Loading…</p>
      <p v-else-if="read.length === 0" class="m-0 text-text-muted">Nothing in the last 7 days.</p>
      <div v-for="(l, i) in read" v-else :key="i" class="grid grid-cols-[auto_3rem_minmax(0,1fr)] gap-x-3 py-px">
        <time class="text-dim" :datetime="l.at">{{ time.format(new Date(l.at)) }}</time>
        <span class="uppercase" :class="level[l.level ?? ''] ?? 'text-dim'">{{ l.level ?? '' }}</span>
        <span class="break-words whitespace-pre-wrap">
          {{ l.message }}<template v-for="[k, v] in l.fields" :key="k"><span class="text-dim"> {{ k }}=</span><span class="text-text-muted">{{ v }}</span></template>
        </span>
      </div>
    </div>
  </div>
</template>
