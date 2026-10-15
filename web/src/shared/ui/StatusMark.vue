<script setup lang="ts">
/** A release's state as a mark. Its word is for screen readers and hover. */
const { state } = defineProps<{ state: 'live' | 'rolling' | 'failed' | 'past' }>()
const word = { live: 'Running', rolling: 'Rolling out', failed: 'Failed', past: 'Not running' }
</script>

<template>
  <span class="inline-flex size-4 shrink-0 items-center justify-center" :title="word[state]">
    <span v-if="state === 'live'" class="relative flex size-2.5">
      <span class="absolute inset-0 animate-ping rounded-full bg-accent opacity-60 motion-reduce:animate-none" />
      <span class="relative size-2.5 rounded-full bg-accent" />
    </span>
    <svg v-else-if="state === 'rolling'" width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" class="animate-spin text-text-primary motion-reduce:animate-none">
      <circle cx="8" cy="8" r="6" fill="none" stroke="currentColor" stroke-opacity="0.25" stroke-width="2" />
      <path d="M8 2a6 6 0 0 1 6 6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
    </svg>
    <svg v-else-if="state === 'failed'" width="12" height="12" viewBox="0 0 12 12" aria-hidden="true" class="text-crit">
      <path d="M2 2l8 8M10 2 2 10" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
    </svg>
    <span v-else class="size-2.5 rounded-full border border-dim" />
    <span class="sr-only">{{ word[state] }}</span>
  </span>
</template>
