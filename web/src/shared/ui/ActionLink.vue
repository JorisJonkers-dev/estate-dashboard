<script setup lang="ts">
import { computed } from 'vue'

/** A square button that goes somewhere: in the app through the router, elsewhere in a new tab. */
const props = defineProps<{ to: string; primary?: boolean }>()
const inside = computed(() => props.to.startsWith('/'))
const look = computed(() => [
  'inline-flex min-h-10 items-center rounded-md border px-4 text-[15px] font-medium no-underline transition-colors',
  props.primary
    ? 'border-accent bg-accent text-[#1c1c1c] hover:bg-transparent hover:text-accent'
    : 'border-control text-text-primary hover:border-accent hover:text-accent',
])
</script>

<template>
  <RouterLink v-if="inside" :to="to" :class="look"><slot /></RouterLink>
  <a v-else :href="to" target="_blank" rel="noopener" :class="look"><slot /><span class="sr-only"> (opens in a new tab)</span></a>
</template>
