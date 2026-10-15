<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import { getDeclarationOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'

/** What the Project declares at its pinned release. */
const props = defineProps<{ project: string }>()
const declaration = useQuery(computed(() => getDeclarationOptions({ path: { project: props.project } })))
const d = computed(() => declaration.data.value)
const label = 'text-xs font-semibold tracking-[0.12em] text-text-muted uppercase'
</script>

<template>
  <div v-if="d" class="flex flex-col gap-10">
    <section v-for="a in d.applications" :key="a.id" :aria-labelledby="`app-${a.id}`">
      <h2 :id="`app-${a.id}`" class="m-0 mb-3 text-xl font-semibold">{{ a.id }}</h2>
      <table class="w-full border-collapse">
        <thead>
          <tr class="border-b border-line">
            <th v-for="h in ['Process', 'Switchover', 'Replicas']" :key="h" scope="col" class="py-2 pr-4 text-left" :class="label">{{ h }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in a.processes" :key="p.name" class="border-b border-line-soft">
            <th scope="row" class="py-2.5 pr-4 text-left font-medium">{{ p.name }}</th>
            <td class="py-2.5 pr-4 text-text-muted">{{ p.switchover }}</td>
            <td class="py-2.5 text-text-muted">{{ p.replicas }}</td>
          </tr>
        </tbody>
      </table>
      <dl v-if="a.migration" class="m-0 mt-4 grid grid-cols-3 gap-4 text-[15px]">
        <div>
          <dt :class="label">Migration</dt>
          <dd class="m-0">{{ a.migration.runner }}<span v-if="a.migration.identity" class="text-text-muted">, {{ a.migration.identity }}</span></dd>
        </div>
        <div>
          <dt :class="label">Proven against</dt>
          <dd class="m-0">{{ a.migration.provenAgainst ?? 'nothing yet' }}</dd>
        </div>
        <div>
          <dt :class="label">Undo</dt>
          <dd class="m-0" :class="a.migration.undo === 'allowed' ? 'text-ok' : 'text-warn'">{{ a.migration.undo }}<span v-if="a.migration.reason" class="block text-sm text-text-muted">{{ a.migration.reason }}</span></dd>
        </div>
      </dl>
    </section>

    <section v-if="d.rollback.length" aria-labelledby="rollback-h">
      <h2 id="rollback-h" class="m-0 mb-3 text-xl font-semibold">Rollback</h2>
      <p class="m-0 mb-2 text-sm text-text-muted">Before it moves the pin:</p>
      <ol class="m-0 flex flex-col gap-1 pl-5">
        <li v-for="s in d.rollback" :key="s">{{ s }}</li>
      </ol>
    </section>

    <section v-if="d.grants.length" aria-labelledby="grants-h">
      <h2 id="grants-h" class="m-0 mb-3 text-xl font-semibold">Grants</h2>
      <ul class="m-0 flex list-none flex-col gap-1 p-0 font-mono text-[13px]">
        <li v-for="g in d.grants" :key="g.process + g.path"><span class="text-text-muted">{{ g.process }}</span> {{ g.path }} <span class="text-dim">{{ g.access }}</span></li>
      </ul>
    </section>
  </div>
</template>
