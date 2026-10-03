<script setup lang="ts">
import { RouterView } from 'vue-router'
</script>

<template>
  <main class="shell">
    <RouterView />
  </main>
</template>

<style>
body {
  margin: 0;
  font-family: system-ui, sans-serif;
  /* The house dark; the dashboard has no light theme. */
  color: #e8e8e8;
  background: #1e1e1e;
}
.shell {
  max-width: 40rem;
  margin: 0 auto;
  padding: 1.5rem 1rem;
}
</style>
