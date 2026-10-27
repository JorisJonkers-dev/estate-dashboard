<script setup lang="ts">
import { computed } from 'vue'
import { useNow } from '../../../infrastructure/useNow'
import { explain } from '../../../shared/glossary'
import { toneText } from '../../../shared/tone'
import EmptyPlate from '../../../shared/ui/EmptyPlate.vue'
import HeadBand from '../../../shared/ui/HeadBand.vue'
import TermHint from '../../../shared/ui/TermHint.vue'
import { useOverview } from '../composables/useOverview'
import { age } from '../domain/overview'
import ProjectsList from './ProjectsList.vue'

const { isLoading, projects, health, needs, unread } = useOverview()
const now = useNow()

const onEstate = computed(() => projects.value.filter((p) => p.path === 'estate').length)
const isExternal = (href: string) => href.startsWith('https://')
</script>

<template>
  <div class="flex flex-col gap-10">
    <HeadBand eyebrow="Production" title="Overview">
      {{ projects.length }} projects, {{ onEstate }} on the estate path
      <template #facts>
        <dl aria-label="Health" class="m-0 grid grid-cols-2 border-t border-line md:grid-cols-4">
          <div
            v-for="(h, i) in health"
            :key="h.name"
            class="flex flex-col gap-0.5 border-line py-4 pr-4 md:px-6 md:first:pl-0"
            :class="{ 'border-l pl-4': i % 2 === 1, 'md:border-l md:pl-6': i > 0, 'border-t md:border-t-0': i > 1 }"
          >
            <dt class="text-xs font-semibold tracking-[0.12em] text-text-muted uppercase">
              <TermHint :text="explain(h.name.toLowerCase())" term>{{ h.name }}</TermHint>
            </dt>
            <dd class="m-0">
              <span class="block text-xl font-semibold md:text-2xl" :class="toneText[h.tone]">{{ h.status }}</span>
              <span class="block text-sm text-text-muted">{{ h.detail }}</span>
            </dd>
          </div>
        </dl>
      </template>
    </HeadBand>

    <p v-if="unread.length > 0" role="status" class="-mt-4 mb-0 text-[15px] text-warn">
      Couldn't read <template v-for="(u, i) in unread" :key="u.name">{{ i > 0 ? ', ' : '' }}{{ u.name }}</template
      >.
      <span class="text-text-muted"
        ><template v-for="(u, i) in unread" :key="u.name">{{ i > 0 ? ' ' : '' }}{{ u.why }}</template></span
      >
    </p>

    <div class="flex flex-col gap-10 lg:grid lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)] lg:items-start lg:gap-12">
      <ProjectsList :projects="projects" :loading="isLoading" />

      <section aria-labelledby="needs-h" class="-order-1 lg:order-none">
        <div class="mb-3 flex items-end justify-between">
          <h2 id="needs-h" class="m-0 text-xl font-semibold">Needs you</h2>
          <RouterLink to="/alerts" class="text-[15px]">All alerts</RouterLink>
        </div>
        <EmptyPlate v-if="needs.length === 0" title="All clear">Nothing is firing or stuck.</EmptyPlate>
        <ul v-else class="scroll-pane m-0 list-none border-t border-line p-0">
          <li
            v-for="(n, i) in needs"
            :key="n.key"
            class="rise flex flex-col gap-0.5 border-b border-line-soft py-3"
            :style="{ '--i': i }"
          >
            <div class="flex items-center gap-2 text-[13px]">
              <span class="size-2 rounded-full bg-current" :class="toneText[n.tone]" aria-hidden="true" />
              <span class="font-semibold" :class="toneText[n.tone]">{{ n.severity }}</span>
              <span class="text-text-muted">{{ n.class }}</span>
              <time class="ml-auto text-text-muted" :datetime="n.since">{{ age(n.since, now) }}</time>
            </div>
            <a v-if="isExternal(n.href)" :href="n.href" class="font-medium no-underline hover:underline">{{
              n.title
            }}</a>
            <RouterLink v-else :to="n.href" class="font-medium no-underline hover:underline">{{ n.title }}</RouterLink>
            <span class="text-sm text-text-muted">{{ n.where }}</span>
            <span v-if="n.actions" class="flex flex-wrap gap-x-4 gap-y-1 text-sm">
              <a v-for="a in n.actions" :key="a.label" :href="a.href">{{ a.label }}</a>
            </span>
          </li>
        </ul>
      </section>
    </div>
  </div>
</template>
