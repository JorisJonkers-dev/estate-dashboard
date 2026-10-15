<script setup lang="ts">
import { toneText, type Tone } from '../tone'
import TermHint from './TermHint.vue'

/** A state word in an outlined pill, with an optional hint. */
defineProps<{ tone: Tone; hint?: string | undefined }>()
</script>

<template>
  <TermHint :text="hint">
    <span :tabindex="hint ? 0 : undefined" class="inline-block rounded-full border border-current px-2.5 py-px text-[13px] font-medium whitespace-nowrap" :class="[toneText[tone], { 'cursor-help': hint }]">
      <slot />
    </span>
  </TermHint>
</template>
