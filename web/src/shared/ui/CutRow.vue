<script setup lang="ts">
/**
 * One row of a list that goes somewhere: cut on both sides at the lean of the bar that slides in
 * on hover, an arrow at its end.
 */
defineProps<{ to: string; label: string }>()
</script>

<template>
  <RouterLink :to="to" class="cut-row cut grid items-center gap-x-5 text-text-primary no-underline hover:text-text-primary" :aria-label="label">
    <slot />
    <svg aria-hidden="true" class="go hidden md:block" width="20" height="12" viewBox="0 0 20 12" fill="none">
      <path d="M0 6h17M13 1.5 18.5 6 13 10.5" stroke="currentColor" stroke-width="1.4" />
    </svg>
  </RouterLink>
</template>

<style scoped>
.cut-row {
  --row-h: 4rem;
  --cut: calc(var(--row-h) * 0.2126);
  position: relative;
  min-height: var(--row-h);
  padding: 0.6rem calc(var(--cut) + 0.8rem) 0.6rem calc(var(--cut) + 1rem);
  background: var(--color-band);
  transition: background-color 220ms ease;
}
.cut-row::before {
  content: '';
  position: absolute;
  top: 0.8rem;
  bottom: 0.8rem;
  left: calc(var(--cut) * 0.5 + 0.35rem);
  width: 3px;
  background: var(--color-accent);
  transform: skewX(-12deg);
  scale: 1 0;
  transition: scale 320ms var(--ease-out-quint);
}
.cut-row:hover,
.cut-row:focus-visible {
  background-color: var(--color-panel);
}
.cut-row:hover::before,
.cut-row:focus-visible::before {
  scale: 1 1;
}
.go {
  color: var(--color-text-muted);
  transition: translate 320ms var(--ease-out-quint), color 220ms ease;
}
.cut-row:hover .go {
  translate: 4px 0;
  color: var(--color-accent);
}
@media (prefers-reduced-motion: reduce) {
  .cut-row::before,
  .go {
    transition: none;
  }
}
</style>
