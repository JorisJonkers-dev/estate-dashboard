<script setup lang="ts">
/** Empty state: a mark, a title and a line. */
defineProps<{ title: string; mark?: 'tick' | 'quiet' }>()
</script>

<template>
  <div class="flex items-start gap-4 rounded-md border border-line-soft bg-band px-6 py-5">
    <svg v-if="mark !== 'quiet'" aria-hidden="true" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="square" class="mt-0.5 shrink-0 text-accent">
      <path d="M4 12.5 9.5 18 20 6.5" />
    </svg>
    <span v-else aria-hidden="true" class="mt-2.5 h-px w-5 shrink-0 bg-text-muted" />
    <div>
      <p class="m-0 font-semibold">{{ title }}</p>
      <p class="m-0 mt-0.5 text-text-muted"><slot /></p>
    </div>
  </div>
</template>
