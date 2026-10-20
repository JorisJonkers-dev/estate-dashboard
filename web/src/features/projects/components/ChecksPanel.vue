<script setup lang="ts">
import type { Check } from '../../../infrastructure/api/types.gen'
import { toneText } from '../../../shared/tone'
import StatusStrip from '../../../shared/ui/StatusStrip.vue'
import { age } from '../../overview/domain/overview'
import { probeBlocks } from '../domain/delivery'
import { checkHealth } from '../domain/health'

/** The uptime checks that probe a Project: each one's latest probe, and its last 60 as a strip. */
defineProps<{ project: string; checks: Check[]; loading: boolean; unreadable: boolean; now: number }>()
</script>

<template>
  <section aria-labelledby="status-h">
    <h2 id="status-h" class="m-0 mb-3 text-xl font-semibold">Status</h2>
    <p v-if="loading" class="text-text-muted">Loading…</p>
    <p v-else-if="unreadable" class="text-text-muted">Not read: Gatus did not answer.</p>
    <p v-else-if="checks.length === 0" class="text-text-muted">No uptime check probes {{ project }}.</p>
    <ul v-else class="m-0 list-none border-t border-line p-0">
      <li v-for="c in checks" :key="c.key" class="flex flex-col gap-1 border-b border-line-soft py-3">
        <span class="flex items-baseline justify-between gap-3">
          <b
            class="truncate font-semibold"
            :class="checkHealth(c, now).tone === 'ok' ? '' : toneText[checkHealth(c, now).tone]"
            >{{ c.name }}</b
          >
          <span class="shrink-0 text-sm text-text-muted"
            ><template v-if="c.status">HTTP {{ c.status }}</template
            ><template v-if="c.durationMs !== undefined"> · {{ c.durationMs }} ms</template
            ><template v-if="c.at">
              · <time :datetime="c.at">{{ age(c.at, now) }} ago</time></template
            ></span
          >
        </span>
        <span v-if="c.host" class="truncate font-mono text-xs text-text-muted" :title="c.host">{{ c.host }}</span>
        <span
          v-if="checkHealth(c, now).detail"
          class="text-sm break-words"
          :class="toneText[checkHealth(c, now).tone]"
          >{{ checkHealth(c, now).detail }}</span
        >
        <StatusStrip
          v-if="c.recent.length > 0"
          class="mt-1.5"
          :blocks="probeBlocks(c.recent)"
          :label="`${String(c.recent.filter(Boolean).length)} of the last ${String(c.recent.length)} probes of ${c.name} succeeded`"
        />
      </li>
    </ul>
  </section>
</template>
