<script setup lang="ts">
defineProps<{ wide?: boolean }>()
</script>

<template>
  <div
    class="flex min-h-screen items-center justify-center bg-[#141414] px-4 py-5 font-sans text-text-primary sm:px-8 sm:py-16"
  >
    <main
      class="flex w-full flex-col gap-5 rounded-md border border-line bg-surface-dark px-6 py-9 pt-9! sm:px-12 sm:py-11 sm:pt-11!"
      :class="wide ? 'max-w-[480px]' : 'max-w-[440px]'"
    >
      <div class="flex items-center gap-3">
        <svg width="34" height="34" viewBox="0 0 26 26" aria-hidden="true">
          <rect x="1" y="1" width="24" height="24" fill="none" stroke="#EDEDED" stroke-width="2" />
          <path d="M1 13h24M13 1v24" stroke="#EDEDED" stroke-width="2" />
          <rect x="14" y="14" width="10" height="10" fill="#A8FF00" />
        </svg>
        <span class="text-[22px] font-semibold tracking-wider">estate</span>
      </div>
      <slot />
    </main>
  </div>
</template>
