<script setup lang="ts">
import { computed } from 'vue'
import { useNow } from '../../../infrastructure/useNow'
import { toneText } from '../../../shared/tone'
import EmptyPlate from '../../../shared/ui/EmptyPlate.vue'
import HeadBand from '../../../shared/ui/HeadBand.vue'
import { useOverview } from '../composables/useOverview'
import { age } from '../domain/overview'
import ProjectsList from './ProjectsList.vue'

const { isLoading, projects, health, needs, unread } = useOverview()
const now = useNow()

const onEstate = computed(() => projects.value.filter((p) => p.path === 'estate').length)
const isExternal = (href: string) => href.startsWith('https://')
</script>

<template>
  <div class="flex flex-col gap-8 md:gap-10">
    <HeadBand eyebrow="Production" title="Overview">
      {{ projects.length }} projects · {{ onEstate }} on the estate path
      <template #facts>
        <dl aria-label="Health" class="m-0 grid grid-cols-2 gap-1.5 md:grid-cols-4 md:gap-0">
          <div v-for="(h, i) in health" :key="h.name" class="slice cut flex flex-col gap-1 px-5 py-3 md:px-9 md:py-4" :class="[toneText[h.tone], { 'md:-ml-4': i > 0 }]">
            <dt class="text-[10px] font-semibold tracking-[0.25em] text-text-muted uppercase md:text-[11px] md:tracking-[0.3em]">{{ h.name }}</dt>
            <dd class="m-0">
              <span class="block text-lg leading-tight font-semibold md:text-2xl">{{ h.status }}</span>
              <span class="block text-xs text-text-muted md:text-sm">{{ h.detail }}</span>
            </dd>
          </div>
        </dl>
      </template>
    </HeadBand>

    <p v-if="unread.length > 0" role="status" class="m-0 text-[15px] text-warn">
      Couldn't read <template v-for="(u, i) in unread" :key="u.name">{{ i > 0 ? ', ' : '' }}{{ u.name }}</template>.
      <span class="text-text-muted"><template v-for="(u, i) in unread" :key="u.name">{{ i > 0 ? ' ' : '' }}{{ u.why }}</template></span>
    </p>

    <div class="flex flex-col gap-8 md:grid md:grid-cols-[minmax(0,2fr)_minmax(0,1fr)] md:items-start md:gap-10">
      <ProjectsList :projects="projects" :loading="isLoading" />

      <section aria-labelledby="needs-h" class="-order-1 md:order-none">
        <div class="mb-4 flex items-end justify-between">
          <h2 id="needs-h" class="m-0 text-2xl font-semibold">Needs you</h2>
          <RouterLink to="/alerts" class="text-[15px]">All alerts</RouterLink>
        </div>
        <EmptyPlate v-if="needs.length === 0" title="All clear">Nothing is firing or stuck.</EmptyPlate>
        <ul v-else class="m-0 flex list-none flex-col gap-1.5 p-0">
          <li v-for="(n, i) in needs" :key="n.key" class="need rise relative flex flex-col gap-1 py-3.5 pr-4 pl-6" :class="toneText[n.tone]" :style="{ '--i': i }">
            <div class="flex items-center gap-2 text-[13px]">
              <span class="font-semibold tracking-wide uppercase">{{ n.severity }}</span>
              <span class="text-text-muted">{{ n.class }}</span>
              <time class="ml-auto text-text-muted" :datetime="n.since">{{ age(n.since, now) }}</time>
            </div>
            <a v-if="isExternal(n.href)" :href="n.href" class="font-semibold text-text-primary no-underline hover:underline">{{ n.title }}</a>
            <RouterLink v-else :to="n.href" class="font-semibold text-text-primary no-underline hover:underline">{{ n.title }}</RouterLink>
            <span class="text-sm text-text-muted">{{ n.where }}</span>
          </li>
        </ul>
      </section>
    </div>
  </div>
</template>

<style scoped>
/* A health slice: its tone washed from the top left over the band, the slices interlocking. */
.slice {
  --cut: 0.7rem;
  background:
    linear-gradient(150deg, color-mix(in oklab, currentColor 14%, transparent), transparent 70%),
    var(--color-panel);
}
@media (min-width: 48rem) {
  .slice {
    --cut: 1.1rem;
  }
}
/* A need: its tone as a leaning bar at the left, over a faint wash of it. */
.need {
  background: linear-gradient(90deg, color-mix(in oklab, currentColor 10%, transparent), transparent 70%), var(--color-band);
}
.need::before {
  content: '';
  position: absolute;
  top: 0.75rem;
  bottom: 0.75rem;
  left: 0.6rem;
  width: 3px;
  background: currentColor;
  transform: skewX(-12deg);
}
</style>
