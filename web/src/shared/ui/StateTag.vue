<script setup lang="ts">
import { toneText, type Tone } from '../tone'

/** A word of state: ruled in its own colour on a faint wash of it, cut on the lean, never filled. */
defineProps<{ tone: Tone }>()
</script>

<template>
  <span class="state-tag cut inline-block px-3 py-0.5 text-[13px] font-semibold tracking-wide whitespace-nowrap" :class="toneText[tone]">
    <slot />
  </span>
</template>

<style scoped>
.state-tag {
  --cut: 0.4rem;
  background: color-mix(in oklab, currentColor 13%, transparent);
  box-shadow: inset 0 -2px 0 currentColor;
}
</style>
