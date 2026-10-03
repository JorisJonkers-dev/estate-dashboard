<script setup lang="ts">
import { useAlertHistory } from '../composables/useAlertHistory'
import { describeEvent } from '../domain/eventText'

const { events, isLoading, loadError } = useAlertHistory()

const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })
</script>

<template>
  <section class="history">
    <h1>Alert history</h1>

    <p v-if="isLoading">Loading the history…</p>
    <p v-else-if="loadError" role="alert" class="error">{{ loadError }}</p>
    <p v-else-if="events.length === 0">No alert has been recorded yet.</p>
    <ul v-else aria-label="Alert history" class="list">
      <li v-for="event in events" :key="event.id">
        <span class="name">{{ event.name }}</span>
        <span class="state" :data-status="event.status">{{ describeEvent(event) }}</span>
        <time :datetime="event.observedAt">{{ dateFormat.format(new Date(event.observedAt)) }}</time>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.history {
  display: grid;
  gap: 1rem;
}
.error {
  color: #ff9e94;
}
.list {
  display: grid;
  padding: 0;
  margin: 0;
  list-style: none;
}
.list li {
  display: grid;
  grid-template-columns: 1fr auto auto;
  gap: 1rem;
  padding: 0.625rem 0;
  border-bottom: 1px solid #3a3a3a;
}
.state,
time {
  color: #b8b8b8;
  white-space: nowrap;
}
</style>
