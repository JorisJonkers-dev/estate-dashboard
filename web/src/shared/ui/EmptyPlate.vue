<script setup lang="ts">
/** A designed nothing: a mark, a line that says what is true, and what to expect. */
defineProps<{ title: string; mark?: 'tick' | 'quiet' }>()
</script>

<template>
  <div class="plate cut relative flex items-center gap-5 overflow-hidden px-8 py-7">
    <svg v-if="mark !== 'quiet'" aria-hidden="true" width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="square" class="shrink-0 text-accent">
      <path d="M4 12.5 9.5 18 20 6.5" />
    </svg>
    <svg v-else aria-hidden="true" width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="shrink-0 text-text-muted">
      <path d="M3 12h4M10 12h4M17 12h4" />
    </svg>
    <div>
      <p class="m-0 text-xl font-semibold">{{ title }}</p>
      <p class="m-0 mt-1 text-text-muted"><slot /></p>
    </div>
  </div>
</template>

<style scoped>
.plate {
  --cut: 1.4rem;
  background:
    linear-gradient(150deg, color-mix(in oklab, var(--color-accent) 7%, transparent), transparent 55%),
    var(--color-band);
}
</style>
