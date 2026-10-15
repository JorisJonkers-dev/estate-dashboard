<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import { getReleaseNotesOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { describeError } from '../../alerts/composables/useAlertHistory'
import { readNotes } from '../domain/notes'

/** A release's notes, read from its GitHub Release and shown here. */
const props = defineProps<{ repository: string; version: string }>()
const notes = useQuery(computed(() => getReleaseNotesOptions({ query: { repository: props.repository, version: props.version } })))
const blocks = computed(() => readNotes(notes.data.value?.body ?? ''))
const failure = computed(() => (notes.error.value ? describeError(notes.error.value) : null))
</script>

<template>
  <div class="px-6 py-5">
    <p v-if="notes.isPending.value" class="m-0 text-text-muted">Loading…</p>
    <p v-else-if="failure" class="m-0 text-text-muted">{{ failure }}</p>
    <p v-else-if="blocks.length === 0" class="m-0 text-text-muted">This release has no notes.</p>
    <template v-for="(b, i) in blocks" v-else :key="i">
      <h3 v-if="b.kind === 'heading'" class="mt-5 mb-2 text-sm font-semibold tracking-[0.12em] text-text-muted uppercase first:mt-0">
        <template v-for="(s, j) in b.spans" :key="j">{{ s.text }}</template>
      </h3>
      <p v-else class="my-1.5" :class="{ 'flex gap-2': b.kind === 'item' }">
        <span v-if="b.kind === 'item'" class="mt-2 size-1.5 shrink-0 rounded-full bg-text-muted" aria-hidden="true" />
        <span><template v-for="(s, j) in b.spans" :key="j"><a v-if="s.href" :href="s.href">{{ s.text }}</a><template v-else>{{ s.text }}</template></template></span>
      </p>
    </template>
  </div>
</template>
