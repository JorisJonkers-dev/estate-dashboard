<script setup lang="ts">
import { glossary } from '../../../shared/glossary'
import { toneText } from '../../../shared/tone'
import StateTag from '../../../shared/ui/StateTag.vue'
import TermHint from '../../../shared/ui/TermHint.vue'
import type { StackHealth } from '../domain/health'

/** A stack's health in one word, and the three parts it is made of. */
defineProps<{ health: StackHealth; loading: boolean }>()
const label = 'text-xs font-semibold tracking-[0.12em] text-text-muted uppercase'
</script>

<template>
  <section aria-labelledby="health-h">
    <h2 id="health-h" class="m-0 mb-3 flex items-center gap-3 text-xl font-semibold">
      Health
      <StateTag v-if="!loading" :tone="health.tone" :hint="glossary.stackHealth">{{ health.word }}</StateTag>
    </h2>
    <p v-if="loading" class="m-0 text-text-muted">Loading…</p>
    <dl v-else class="m-0 grid grid-cols-1 gap-x-8 gap-y-3 border-t border-line pt-3 sm:grid-cols-3">
      <div v-for="p in health.parts" :key="p.name" class="min-w-0">
        <dt :class="label">
          <TermHint :text="glossary[p.name]" term>{{ p.name }}</TermHint>
        </dt>
        <dd class="m-0 mt-1 flex items-center gap-2 font-medium" :class="toneText[p.tone]">
          <span class="size-2 shrink-0 rounded-full bg-current" aria-hidden="true" />{{ p.word }}
        </dd>
        <dd v-if="p.detail" class="m-0 mt-0.5 text-sm break-words text-text-muted">{{ p.detail }}</dd>
      </div>
    </dl>
  </section>
</template>
