<script setup lang="ts">
import { ref } from 'vue'
import type { Stream } from '../../../infrastructure/api/types.gen'
import { toneText } from '../../../shared/tone'
import { age } from '../../overview/domain/overview'
import { byProcess, ofRelease } from '../domain/logs'
import { stageTone } from '../domain/project'
import type { ReleaseView } from '../domain/releases'

/** Each release as a row that folds out: what happened, and the containers it ran. The latest is open. */
const props = defineProps<{
  project: string
  releases: ReleaseView[]
  streams: Stream[]
  /** Each container's version in the release running now, by container. */
  versions: Map<string, string>
  current?: string | undefined
  changelog: boolean
  loading: boolean
  now: number
}>()
const emit = defineEmits<{ notes: [version: string] }>()

// The latest release is open until a row is folded; from then on the rows are as they were left.
const touched = ref(false)
const open = ref(new Set<number>())
const opened = (key: number) => (touched.value ? open.value.has(key) : props.releases[0]?.key === key)
function toggle(key: number) {
  const next = new Set(props.releases.filter((r) => opened(r.key)).map((r) => r.key))
  if (next.has(key)) next.delete(key)
  else next.add(key)
  open.value = next
  touched.value = true
}

const dot: Record<ReleaseView['state'], string> = {
  live: 'bg-accent',
  rolling: 'bg-info',
  failed: 'bg-crit',
  past: 'border border-dim bg-transparent',
}
const containersOf = (version: string) =>
  byProcess(ofRelease(props.streams, version), props.now).map((p) => ({
    name: p.process,
    version: version === props.current ? props.versions.get(p.process) : undefined,
  }))

const label = 'mb-1.5 text-xs font-semibold tracking-[0.12em] text-text-muted uppercase'
const quiet =
  'cursor-pointer border-0 bg-transparent p-0 font-sans text-sm text-text-muted underline decoration-line underline-offset-4 hover:text-accent'
const button =
  'inline-flex min-h-8 cursor-pointer items-center rounded-md border border-control bg-transparent px-3 font-sans text-sm text-text-primary no-underline transition-colors hover:border-accent hover:text-accent max-md:min-h-9'
</script>

<template>
  <section aria-labelledby="releases-h">
    <h2 id="releases-h" class="m-0 mb-3 text-xl font-semibold">Releases</h2>
    <p v-if="loading" class="text-text-muted">Loading…</p>
    <p v-else-if="releases.length === 0" class="text-text-muted">
      No releases yet. The next one shows up once composition pins it.
    </p>
    <ol v-else class="m-0 list-none border-t border-line p-0">
      <li v-for="r in releases" :key="r.key" class="border-b border-line-soft">
        <div class="flex items-center gap-3">
          <button
            type="button"
            class="flex min-h-12 min-w-0 grow cursor-pointer items-center gap-3 border-0 bg-transparent p-0 text-left font-sans text-base text-text-primary"
            :aria-expanded="opened(r.key)"
            :aria-controls="`release-${String(r.key)}`"
            @click="toggle(r.key)"
          >
            <svg
              width="12"
              height="12"
              viewBox="0 0 12 12"
              fill="none"
              stroke="currentColor"
              stroke-width="1.8"
              aria-hidden="true"
              class="shrink-0 text-text-muted transition-transform"
              :class="{ 'rotate-90': opened(r.key) }"
            >
              <path d="M4 2l4 4-4 4" />
            </svg>
            <span class="size-2.5 shrink-0 rounded-full" :class="dot[r.state]" aria-hidden="true" />
            <span class="w-20 shrink-0 font-mono font-semibold">{{ r.version }}</span>
            <time class="w-16 shrink-0 text-sm text-text-muted max-md:hidden" :datetime="r.at"
              >{{ age(r.at, now) }} ago</time
            >
            <span class="min-w-0 truncate text-sm text-text-muted">{{ r.stages[0]?.description ?? '' }}</span>
          </button>
          <span class="flex shrink-0 items-center gap-3.5">
            <button v-if="changelog" type="button" :class="[quiet, 'max-md:hidden']" @click="emit('notes', r.version)">
              Changelog
            </button>
            <a v-if="r.run" :href="r.run" target="_blank" rel="noopener" :class="[quiet, 'max-md:hidden']"
              >Run<span class="sr-only"> (opens in a new tab)</span></a
            >
            <RouterLink :to="{ path: `/projects/${project}/rollout`, query: { version: r.version } }" :class="button"
              >Rollout<span class="sr-only"> of {{ r.version }}</span></RouterLink
            >
          </span>
        </div>
        <div
          v-show="opened(r.key)"
          :id="`release-${String(r.key)}`"
          class="mb-4 grid gap-6 pl-[3.25rem] md:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)]"
        >
          <div>
            <p :class="label">What happened</p>
            <ol class="m-0 list-none p-0">
              <li
                v-for="s in [...r.stages].reverse()"
                :key="s.at + s.description"
                class="flex items-baseline gap-2.5 py-0.5 text-sm"
              >
                <time class="w-14 shrink-0 text-text-muted" :datetime="s.at">{{ age(s.at, now) }}</time>
                <span
                  class="size-1.5 shrink-0 rounded-full bg-current"
                  :class="toneText[stageTone(s)]"
                  aria-hidden="true"
                />
                <span class="min-w-0">{{ s.description }}</span>
              </li>
            </ol>
          </div>
          <div>
            <p :class="label">Containers in this release</p>
            <p v-if="containersOf(r.version).length === 0" class="m-0 text-sm text-text-muted">
              No container wrote under this release.
            </p>
            <dl v-else class="m-0 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-0.5 text-sm">
              <template v-for="c in containersOf(r.version)" :key="c.name">
                <dt>{{ c.name }}</dt>
                <dd class="m-0 font-mono text-text-muted">{{ c.version ?? '—' }}</dd>
              </template>
            </dl>
          </div>
        </div>
      </li>
    </ol>
  </section>
</template>
