<script setup lang="ts">
import type { Mark, Segment, Track } from '../domain/rollout'

/**
 * One-shot work and sidecars over a release, drawn up to the moment shown: a span while it works,
 * striped, pulsing while it still does; a mark where it ended; nothing after, so work that is done
 * reads as gone rather than down. Never a health check strip, never a traffic bar.
 */
const props = defineProps<{ track: Track; length: number; label: string }>()

const share = (s: number) => Math.min(100, Math.max(0, (s / Math.max(1, props.length)) * 100))
const left = (s: number) => `${String(share(s))}%`
const width = (s: Segment) => `max(2px, ${String(share(s.to) - share(s.from))}%)`

const fill: Record<Segment['state'], string> = {
  waiting: 'top-1/2 h-0 -translate-y-1/2 border-t border-dashed border-dim',
  running:
    'inset-y-0.5 rounded-sm bg-info/60 [background-image:repeating-linear-gradient(-45deg,transparent_0_3px,rgb(0_0_0/0.35)_3px_6px)]',
  ready: 'inset-y-0.5 rounded-sm bg-ok/70',
  down: 'inset-y-0.5 rounded-sm bg-crit/70',
}
const markTone: Record<Mark['outcome'], string> = { succeeded: 'text-ok', failed: 'text-crit', unknown: 'text-dim' }
</script>

<template>
  <div role="img" :aria-label="label" class="relative h-3.5 w-full">
    <span class="absolute inset-x-0 top-1/2 h-px -translate-y-1/2 bg-line-soft" aria-hidden="true" />
    <span
      v-for="(s, i) in track.segments"
      :key="`s${String(i)}`"
      data-segment
      :data-state="s.state"
      class="absolute"
      :class="[fill[s.state], { 'motion-safe:animate-pulse': s.open && s.state === 'running' }]"
      :style="{ left: left(s.from), width: width(s) }"
      aria-hidden="true"
    />
    <span
      v-for="(m, i) in track.marks"
      :key="`m${String(i)}`"
      data-mark
      :data-outcome="m.outcome"
      class="absolute top-1/2 flex size-3.5 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full border border-current bg-surface-dark"
      :class="markTone[m.outcome]"
      :style="{ left: left(m.at) }"
      aria-hidden="true"
    >
      <svg width="8" height="8" viewBox="0 0 8 8" fill="none" stroke="currentColor" stroke-width="1.5">
        <path v-if="m.outcome === 'succeeded'" d="M1.5 4.2 3.2 5.8 6.5 2.2" />
        <path v-else-if="m.outcome === 'failed'" d="M2 2l4 4M6 2 2 6" />
        <circle v-else cx="4" cy="4" r="1" fill="currentColor" stroke="none" />
      </svg>
    </span>
  </div>
</template>
