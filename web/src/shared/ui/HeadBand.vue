<script setup lang="ts">
import BandRule from './BandRule.vue'

/**
 * The head of a page: an eyebrow, the heading and a line under it, on a band washed from the top
 * left in the page's accent, over the hatch. A seam closes it. Facts and actions go in its slots.
 */
const { eyebrow, title, accent = 'var(--color-accent)' } = defineProps<{ eyebrow: string; title: string; accent?: string }>()
</script>

<template>
  <header class="relative -mx-4 md:-mx-8">
    <div class="head hatch relative overflow-hidden px-4 pt-7 pb-6 md:px-8 md:pt-9 md:pb-8" :style="{ '--accent': accent }">
      <span aria-hidden="true" class="blob" />
      <div class="relative flex flex-wrap items-end justify-between gap-6">
        <div class="min-w-0">
          <p class="rise m-0 text-[11px] font-semibold tracking-[0.3em] text-accent uppercase">{{ eyebrow }}</p>
          <h1 class="rise m-0 mt-2 text-[30px] leading-[1.05] font-semibold tracking-[0.01em] md:text-[40px]" style="--i: 1">
            {{ title }}<slot name="title" />
          </h1>
          <div class="rise mt-2 text-text-muted" style="--i: 2"><slot /></div>
        </div>
        <div v-if="$slots.acts" class="rise flex flex-wrap gap-2.5" style="--i: 3"><slot name="acts" /></div>
      </div>
      <div v-if="$slots.facts" class="rise relative mt-6" style="--i: 3"><slot name="facts" /></div>
    </div>
    <BandRule />
  </header>
</template>

<style scoped>
.head {
  background: linear-gradient(150deg, color-mix(in oklab, var(--accent) 9%, transparent), transparent 62%), var(--color-band);
}
.blob {
  position: absolute;
  top: -40%;
  right: -6%;
  width: 34rem;
  height: 34rem;
  border-radius: 9999px;
  background: radial-gradient(closest-side, color-mix(in oklab, var(--color-action) 16%, transparent), transparent);
  pointer-events: none;
}
</style>
