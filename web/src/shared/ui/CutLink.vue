<script setup lang="ts">
/**
 * The one button shape: cut on the lean, the accent sweeping in from the left on hover. A path is
 * followed by the router; anything else opens in a new tab with an arrow saying so.
 */
const { to, tone = 'plain' } = defineProps<{ to: string; tone?: 'solid' | 'plain' }>()
const inside = to.startsWith('/')
</script>

<template>
  <RouterLink v-if="inside" :to="to" class="cut-link cut" :class="tone"><span><slot /></span></RouterLink>
  <a v-else :href="to" target="_blank" rel="noopener" class="cut-link cut" :class="tone"><span><slot /> <span aria-hidden="true">↗</span><span class="sr-only">(opens in a new tab)</span></span></a>
</template>

<style scoped>
.cut-link {
  position: relative;
  display: inline-flex;
  align-items: center;
  overflow: hidden;
  min-height: 44px;
  padding: 0 1.35rem;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-decoration: none;
  white-space: nowrap;
  color: var(--color-text-primary);
  background: color-mix(in oklab, var(--color-text-primary) 8%, transparent);
}
.cut-link.solid {
  color: #1c1c1c;
  background: var(--color-action);
}
.cut-link::before {
  content: '';
  position: absolute;
  inset: 0;
  background: var(--color-action);
  transform-origin: left center;
  scale: 0 1;
  transition: scale 320ms var(--ease-out-quint);
}
.cut-link.solid::before {
  background: var(--color-accent);
}
.cut-link:hover,
.cut-link:focus-visible {
  color: #1c1c1c;
}
.cut-link:hover::before,
.cut-link:focus-visible::before {
  scale: 1 1;
}
.cut-link > span {
  position: relative;
}
@media (prefers-reduced-motion: reduce) {
  .cut-link::before {
    transition: none;
  }
}
</style>
