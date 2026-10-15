<script setup lang="ts">
import { toneText, type Tone } from '../tone'

/** Top of a page: title, facts and actions. A tone adds a faint wash of colour. */
const { eyebrow, title, tone = 'none' } = defineProps<{ eyebrow: string; title: string; tone?: Tone | 'none' }>()
</script>

<template>
  <header class="band -mx-6 border-b border-line lg:-mx-10" :class="tone === 'none' ? 'text-text-muted' : toneText[tone]">
    <div class="px-6 pt-8 pb-7 text-text-primary lg:px-10 lg:pt-10 lg:pb-8">
      <div class="flex flex-wrap items-end justify-between gap-6">
        <div class="min-w-0">
          <p class="m-0 text-xs font-semibold tracking-[0.2em] text-text-muted uppercase">{{ eyebrow }}</p>
          <h1 class="m-0 mt-1.5 text-3xl leading-tight font-semibold text-text-primary lg:text-4xl">{{ title }}<slot name="title" /></h1>
          <div class="mt-2 text-text-muted"><slot /></div>
        </div>
        <div v-if="$slots.acts" class="flex flex-wrap gap-2"><slot name="acts" /></div>
      </div>
      <div v-if="$slots.facts" class="mt-7"><slot name="facts" /></div>
    </div>
  </header>
</template>

<style scoped>
/* A faint tone wash from the left. */
.band {
  background: linear-gradient(90deg, color-mix(in oklab, currentColor 5%, transparent), transparent 45%), var(--color-band);
}
</style>
