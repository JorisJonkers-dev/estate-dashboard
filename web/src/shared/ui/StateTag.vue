<script setup lang="ts">
import { toneText, type Tone } from '../tone'

/** A word of state: an outlined pill in its colour, never filled. */
defineProps<{ tone: Tone }>()
</script>

<template>
  <span class="inline-block rounded-full border border-current px-2.5 py-px text-[13px] font-medium whitespace-nowrap" :class="toneText[tone]">
    <slot />
  </span>
</template>
