<script setup lang="ts">
import { computed, ref } from 'vue'
import type { StripBlock } from '../strip'

/**
 * A frameless row of blocks that share the width they are given: the health timeline, a check's
 * probes, a rollout's health checks. One tab stop; the arrow keys move between blocks, and the
 * block hovered or focused names itself in a tooltip that takes no room.
 */
const props = defineProps<{ blocks: StripBlock[]; label: string }>()

const shown = ref<number | null>(null)
const focused = ref(Math.max(0, props.blocks.length - 1))
const strip = ref<HTMLElement | null>(null)

const colour: Record<StripBlock['tone'], string> = {
  ok: 'bg-ok/70',
  info: 'bg-info/70',
  warn: 'bg-warn',
  crit: 'bg-crit',
  empty: 'bg-line',
}

function show(i: number) {
  focused.value = i
  shown.value = i
}

function move(by: number) {
  focused.value = Math.min(props.blocks.length - 1, Math.max(0, focused.value + by))
  shown.value = focused.value
  strip.value?.querySelectorAll<HTMLElement>('[data-block]')[focused.value]?.focus()
}

const tip = computed(() => (shown.value === null ? undefined : props.blocks[shown.value]))
const tipAt = computed(() =>
  shown.value === null ? 0 : Math.min(92, Math.max(8, ((shown.value + 0.5) / props.blocks.length) * 100)),
)
</script>

<template>
  <div class="relative w-full">
    <div ref="strip" role="toolbar" :aria-label="label" class="flex h-4 w-full items-stretch gap-px">
      <button
        v-for="(b, i) in blocks"
        :key="i"
        type="button"
        data-block
        :tabindex="i === focused ? 0 : -1"
        :aria-label="b.label"
        class="m-0 min-w-px flex-1 cursor-default rounded-[1px] border-0 p-0 outline-offset-1"
        :class="colour[b.tone]"
        @mouseenter="shown = i"
        @mouseleave="shown = null"
        @blur="shown = null"
        @keydown.left.prevent="move(-1)"
        @keydown.right.prevent="move(1)"
        @keydown.home.prevent="move(-blocks.length)"
        @keydown.end.prevent="move(blocks.length)"
        @focus="show(i)"
      />
    </div>
    <span
      v-if="tip"
      role="tooltip"
      class="pointer-events-none absolute bottom-full z-40 mb-1.5 -translate-x-1/2 rounded-md border border-line bg-[#2a2a2a] px-2 py-1 text-xs whitespace-nowrap text-text-primary shadow-lg"
      :style="{ left: `${String(tipAt)}%` }"
      >{{ tip.label }}</span
    >
  </div>
</template>
