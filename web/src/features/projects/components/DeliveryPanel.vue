<script setup lang="ts">
import { toneText } from '../../../shared/tone'
import type { DeliveryFacts } from '../domain/delivery'
import { links } from '../domain/project'

/** Where a Project's delivery stands: its pin, whether Flux has it, and how it switches. */
defineProps<{ project: string; facts: DeliveryFacts }>()
</script>

<template>
  <section aria-labelledby="delivery-h">
    <h2 id="delivery-h" class="m-0 mb-3 text-xl font-semibold">Delivery</h2>
    <dl class="m-0 grid grid-cols-[auto_minmax(0,1fr)] gap-x-5 gap-y-2 border-t border-line pt-3 text-[15px]">
      <dt class="text-text-muted">Pin</dt>
      <dd class="m-0">
        <template v-if="facts.pin"
          ><a :href="links.pin(project)" class="font-mono">{{ facts.pin }}</a
          ><template v-if="facts.release"> · release {{ facts.release }}</template></template
        ><template v-else>nothing pinned</template>
      </dd>
      <dt class="text-text-muted">Fetched</dt>
      <dd class="m-0" :class="toneText[facts.fetched.tone]">{{ facts.fetched.word }}</dd>
      <dt class="text-text-muted">Applied</dt>
      <dd class="m-0">
        <span :class="toneText[facts.applied.tone]">{{ facts.applied.word }}</span
        ><template v-if="facts.after.length"
          >, after <span class="font-mono text-sm">{{ facts.after.join(', ') }}</span></template
        >
      </dd>
      <dt class="text-text-muted">Switches</dt>
      <dd class="m-0">{{ facts.switches }}</dd>
      <template v-if="facts.artifact">
        <dt class="text-text-muted">Artifact</dt>
        <dd class="m-0 truncate font-mono text-sm" :title="facts.artifact">{{ facts.artifact }}</dd>
      </template>
    </dl>
  </section>
</template>
