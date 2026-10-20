<script setup lang="ts">
import type { Check } from '../../../infrastructure/api/types.gen'
import { glossary } from '../../../shared/glossary'
import { toneText } from '../../../shared/tone'
import TermHint from '../../../shared/ui/TermHint.vue'
import { age } from '../../overview/domain/overview'
import { checkHealth } from '../domain/health'

/** The uptime checks that probe a Project, each with its latest probe and its recent ones. */
defineProps<{ project: string; checks: Check[]; loading: boolean; unreadable: boolean; now: number }>()
</script>

<template>
  <section aria-labelledby="online-h">
    <h2 id="online-h" class="m-0 mb-3 text-xl font-semibold">
      <TermHint :text="glossary.check" term>Uptime checks</TermHint>
    </h2>
    <p v-if="loading" class="text-text-muted">Loading…</p>
    <p v-else-if="unreadable" class="text-text-muted">Gatus did not answer.</p>
    <p v-else-if="checks.length === 0" class="text-text-muted">No uptime check probes {{ project }}.</p>
    <ul v-else class="m-0 list-none border-t border-line p-0">
      <li v-for="c in checks" :key="c.key" class="flex flex-col gap-1 border-b border-line-soft py-3">
        <span class="flex items-baseline justify-between gap-3">
          <span class="flex min-w-0 items-center gap-2 font-medium">
            <span
              class="size-2 shrink-0 rounded-full bg-current"
              :class="toneText[checkHealth(c, now).tone]"
              aria-hidden="true"
            />
            <span class="truncate">{{ c.name }}</span>
          </span>
          <span class="shrink-0 font-medium" :class="toneText[checkHealth(c, now).tone]">{{
            checkHealth(c, now).word
          }}</span>
        </span>
        <span v-if="c.host" class="truncate font-mono text-xs text-text-muted" :title="c.host">{{ c.host }}</span>
        <span class="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-text-muted">
          <span v-if="c.status">HTTP {{ c.status }}</span>
          <span v-if="c.durationMs !== undefined">{{ c.durationMs }} ms</span>
          <time v-if="c.at" :datetime="c.at">{{ age(c.at, now) }} ago</time>
          <span v-if="checkHealth(c, now).detail" class="basis-full break-words">{{ checkHealth(c, now).detail }}</span>
        </span>
        <span
          v-if="c.recent.length > 0"
          class="mt-1 flex h-4 items-end gap-px"
          role="img"
          :aria-label="`${c.recent.filter(Boolean).length} of the last ${c.recent.length} probes succeeded`"
        >
          <span
            v-for="(ok, i) in c.recent"
            :key="i"
            class="h-full w-1.5 rounded-sm"
            :class="ok ? 'bg-ok/70' : 'bg-crit'"
          />
        </span>
      </li>
    </ul>
  </section>
</template>
