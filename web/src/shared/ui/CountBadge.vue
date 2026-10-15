<script setup lang="ts">
/** A count riding the heading it counts: a blue pill on the last word's corner. */
defineProps<{ count: number; said: string }>()
</script>

<template>
  <span class="whitespace-nowrap">&#8288;<span
    aria-hidden="true"
    class="relative -ml-1 inline-block min-w-6 -translate-y-2 rounded-full bg-action px-2 py-0.5 text-center align-top text-xs leading-tight font-semibold text-[#1c1c1c]"
  >{{ count }}</span><span class="sr-only">{{ count }} {{ said }}</span></span>
</template>
