<script setup lang="ts">
import { computed, nextTick, useTemplateRef, watch } from 'vue'
import { useLogs } from '../composables/useLogs'
import { streamKey, windows } from '../domain/logs'

const props = defineProps<{ project: string }>()
const { hours, picked, groups, lines, isLoading, linesLoading, unread } = useLogs(computed(() => props.project))

const time = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' })

// Keep the newest line in view as lines arrive.
const view = useTemplateRef<HTMLElement>('view')
watch(lines, async () => {
  await nextTick()
  if (view.value) view.value.scrollTop = view.value.scrollHeight
})
</script>

<template>
  <section aria-labelledby="logs-h">
    <div class="mb-3 flex flex-wrap items-end justify-between gap-4">
      <h2 id="logs-h" class="m-0 text-xl font-semibold">Logs</h2>
      <label class="flex items-center gap-2 text-[15px] text-text-muted">
        From the last
        <select v-model.number="hours" class="min-h-9 rounded-md border border-control bg-surface-dark px-2 font-sans text-text-primary">
          <option v-for="w in windows" :key="w.hours" :value="w.hours">{{ w.label }}</option>
        </select>
      </label>
    </div>

    <p v-if="unread" role="status" class="m-0 text-[15px] text-warn">{{ unread }}</p>
    <p v-else-if="isLoading" class="m-0 text-text-muted">Loading…</p>
    <p v-else-if="groups.length === 0" class="m-0 text-text-muted">No container of {{ project }} wrote logs in this window.</p>
    <div v-else class="grid overflow-hidden rounded-md border border-line md:grid-cols-[14rem_minmax(0,1fr)]">
      <nav aria-label="Containers" class="max-h-96 overflow-y-auto border-b border-line bg-band md:border-r md:border-b-0">
        <div v-for="g in groups" :key="g.name" class="py-2">
          <p class="m-0 px-4 py-1 text-xs font-semibold tracking-[0.12em] text-text-muted uppercase">{{ g.name }}</p>
          <button
            v-for="s in g.streams"
            :key="streamKey(s)"
            type="button"
            :aria-pressed="picked !== null && streamKey(picked) === streamKey(s)"
            class="block w-full cursor-pointer border-0 border-l-2 border-transparent bg-transparent px-4 py-1.5 text-left font-sans text-sm text-nav transition-colors hover:bg-panel hover:text-white aria-pressed:border-accent aria-pressed:bg-panel aria-pressed:text-white"
            @click="picked = s"
          >
            <span class="block truncate font-medium">{{ s.container }}</span>
            <span class="block truncate font-mono text-xs text-dim">{{ s.pod }}</span>
          </button>
        </div>
      </nav>
      <div ref="view" class="max-h-96 overflow-auto bg-[#181818] p-4 font-mono text-xs leading-relaxed" tabindex="0" aria-label="Log lines">
        <p v-if="linesLoading" class="m-0 text-text-muted">Loading…</p>
        <p v-else-if="lines.length === 0" class="m-0 text-text-muted">Nothing in this window.</p>
        <p v-for="(l, i) in lines" v-else :key="i" class="m-0 whitespace-pre-wrap break-all">
          <time class="mr-3 text-dim" :datetime="l.at">{{ time.format(new Date(l.at)) }}</time>{{ l.text }}
        </p>
      </div>
    </div>
  </section>
</template>
