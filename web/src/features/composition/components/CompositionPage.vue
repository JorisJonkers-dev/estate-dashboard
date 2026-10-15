<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { computed, ref } from 'vue'
import { listCompositionRunsOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { CompositionFragment, CompositionRun } from '../../../infrastructure/api/types.gen'
import { useNow } from '../../../infrastructure/useNow'
import type { Tone } from '../../../shared/tone'
import { toneText } from '../../../shared/tone'
import EmptyPlate from '../../../shared/ui/EmptyPlate.vue'
import HeadBand from '../../../shared/ui/HeadBand.vue'
import { describeError } from '../../alerts/composables/useAlertHistory'
import { age } from '../../overview/domain/overview'

const runs = useQuery({ ...listCompositionRunsOptions(), refetchInterval: 60_000 })
const items = computed(() => runs.data.value?.items ?? [])
const failure = computed(() => (runs.error.value ? describeError(runs.error.value) : null))
const now = useNow()

const pickedId = ref<number | null>(null)
const picked = computed(() => items.value.find((r) => r.id === pickedId.value) ?? items.value[0])

const outcome: Record<CompositionRun['outcome'], { word: string; tone: Tone }> = {
  composed: { word: 'All composed', tone: 'ok' },
  isolated: { word: 'Some set aside', tone: 'crit' },
  failed: { word: 'Failed', tone: 'crit' },
}
const fragment: Record<CompositionFragment['outcome'], { word: string; tone: Tone }> = {
  moved: { word: 'Pin moved', tone: 'info' },
  unchanged: { word: 'Unchanged', tone: 'idle' },
  isolated: { word: 'Set aside', tone: 'crit' },
  stale: { word: 'Stale', tone: 'warn' },
  held: { word: 'Held', tone: 'back' },
}
const label = 'text-xs font-semibold tracking-[0.12em] text-text-muted uppercase'
const when = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })
</script>

<template>
  <div class="flex flex-col gap-10">
    <HeadBand eyebrow="Production" title="Composition">
      Each run builds the estate from the latest releases and updates what changed. A refused one is set aside. The rest still compose.
    </HeadBand>

    <EmptyPlate v-if="failure" title="Runs aren't shown yet" mark="quiet">{{ failure }}</EmptyPlate>
    <p v-else-if="runs.isPending.value" class="m-0 text-text-muted">Loading…</p>
    <EmptyPlate v-else-if="items.length === 0" title="No runs yet" mark="quiet">The next run shows up here.</EmptyPlate>
    <div v-else class="flex flex-col gap-8 lg:grid lg:grid-cols-[minmax(0,1fr)_minmax(0,1.7fr)] lg:items-start lg:gap-12">
      <ul aria-label="Runs" class="m-0 list-none border-t border-line p-0">
        <li v-for="r in items" :key="r.id">
          <button
            type="button"
            :aria-pressed="picked?.id === r.id"
            class="flex w-full cursor-pointer items-center justify-between gap-3 border-0 border-b border-l-2 border-line-soft border-l-transparent bg-transparent px-3 py-3 text-left font-sans text-text-primary transition-colors hover:bg-panel aria-pressed:border-l-accent aria-pressed:bg-panel"
            @click="pickedId = r.id"
          >
            <span class="min-w-0">
              <span class="block font-medium">{{ age(r.at, now) }} ago</span>
              <span class="block truncate text-sm text-text-muted">{{ r.trigger }}</span>
            </span>
            <span class="flex items-center gap-2 text-sm font-medium whitespace-nowrap" :class="toneText[outcome[r.outcome].tone]">
              <span class="size-2 rounded-full bg-current" aria-hidden="true" />{{ outcome[r.outcome].word }}
            </span>
          </button>
        </li>
      </ul>

      <section v-if="picked" aria-labelledby="run-h" class="flex flex-col gap-5 lg:border-l lg:border-line lg:pl-8">
        <div class="flex flex-wrap items-baseline justify-between gap-3">
          <h2 id="run-h" class="m-0 text-xl font-semibold">{{ when.format(new Date(picked.at)) }}</h2>
          <a :href="picked.url" target="_blank" rel="noopener" class="inline-flex min-h-9 items-center rounded-md border border-control px-3 text-sm font-medium no-underline hover:border-accent">Workflow run<span class="sr-only"> (opens in a new tab)</span></a>
        </div>
        <dl class="m-0 grid grid-cols-2 gap-4 md:grid-cols-4">
          <div>
            <dt :class="label">Lock</dt>
            <dd class="m-0 font-mono text-sm">{{ picked.lock ?? '' }}</dd>
          </div>
          <div>
            <dt :class="label">Toolkit</dt>
            <dd class="m-0">{{ picked.toolkit ? `deploy-kit ${picked.toolkit}` : '' }}</dd>
          </div>
          <div>
            <dt :class="label">Snapshot</dt>
            <dd class="m-0">{{ picked.snapshotAt ? `${age(picked.snapshotAt, now)} old` : '' }}</dd>
          </div>
          <div>
            <dt :class="label">Pins moved</dt>
            <dd class="m-0">{{ picked.moved.length ? picked.moved.join(', ') : 'none' }}</dd>
          </div>
        </dl>
        <table aria-label="Fragments in this run" class="w-full border-collapse">
          <thead>
            <tr class="border-b border-line">
              <th v-for="h in ['Project', 'Version', 'Outcome', 'Published']" :key="h" scope="col" class="py-2 pr-4 text-left" :class="label">{{ h }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="f in picked.fragments" :key="f.project" class="border-b border-line-soft">
              <th scope="row" class="py-2.5 pr-4 text-left font-medium"><RouterLink :to="`/projects/${f.project}`" class="no-underline hover:underline">{{ f.project }}</RouterLink></th>
              <td class="py-2.5 pr-4">{{ f.version }}</td>
              <td class="py-2.5 pr-4" :class="toneText[fragment[f.outcome].tone]">
                {{ fragment[f.outcome].word }}<span v-if="f.reason" class="block text-sm text-text-muted">{{ f.reason }}</span>
              </td>
              <td class="py-2.5 text-text-muted">{{ age(f.published, now) }}</td>
            </tr>
          </tbody>
        </table>
      </section>
    </div>
  </div>
</template>
