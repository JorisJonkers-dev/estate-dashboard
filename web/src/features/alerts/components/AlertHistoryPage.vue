<script setup lang="ts">
import { useAlertHistory } from '../composables/useAlertHistory'
import { describeEvent } from '../domain/eventText'

const { events, isLoading, loadError } = useAlertHistory()

const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })
</script>

<template>
  <section class="grid gap-4">
    <h1 class="m-0 text-[28px] font-semibold md:text-[34px]">Alert history</h1>

    <p v-if="isLoading" class="m-0">Loading the history…</p>
    <p v-else-if="loadError" role="alert" class="m-0 text-crit">{{ loadError }}</p>
    <p v-else-if="events.length === 0" class="m-0">No alert has been recorded yet.</p>
    <ul v-else aria-label="Alert history" class="m-0 grid list-none p-0">
      <li v-for="event in events" :key="event.id" class="grid grid-cols-[1fr_auto_auto] gap-4 border-b border-line-soft py-2.5">
        <span class="name">{{ event.name }}</span>
        <span class="state whitespace-nowrap text-text-muted" :data-status="event.status">{{ describeEvent(event) }}</span>
        <time class="whitespace-nowrap text-text-muted" :datetime="event.observedAt">{{ dateFormat.format(new Date(event.observedAt)) }}</time>
      </li>
    </ul>
  </section>
</template>
