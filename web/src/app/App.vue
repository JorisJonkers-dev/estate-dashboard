<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuPortal, DropdownMenuRoot, DropdownMenuTrigger } from 'reka-ui'
import { computed, useTemplateRef } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { getSessionOptions, listAlertsOptions } from '../infrastructure/api/@tanstack/vue-query.gen'

const route = useRoute()

/** The pages the nav holds; each screen joins it as it is built. */
const pages = [
  { to: '/', label: 'Overview', names: ['overview'] },
  { to: '/projects', label: 'Projects', names: ['projects', 'project'] },
  { to: '/alerts', label: 'Alerts', names: ['alerts'] },
] as const
const current = (names: readonly string[]) => (names.includes(String(route.name)) ? 'page' : undefined)

// The shell lives on every signed-in page; outside one it asks the API nothing.
const signedInPage = computed(() => !route.meta.public)
const session = useQuery(computed(() => ({ ...getSessionOptions(), enabled: signedInPage.value })))
const alerts = useQuery(computed(() => ({ ...listAlertsOptions(), enabled: signedInPage.value, refetchInterval: 30_000 })))
const firing = computed(() => (alerts.data.value?.items ?? []).filter((a) => a.silencedBy.length === 0).length)

// Signing out is a form post the server answers with a redirect; the menu item submits it.
const signOut = useTemplateRef<HTMLFormElement>('signOut')
</script>

<template>
  <RouterView v-if="route.meta.public" />
  <div v-else class="flex min-h-screen flex-col">
    <header class="border-b border-line">
      <div class="flex h-14 items-stretch justify-between gap-4 px-6 md:justify-start md:gap-12 lg:px-10">
        <RouterLink to="/" aria-label="estate, overview" class="flex items-center gap-2.5 text-lg font-semibold tracking-wider text-text-primary no-underline hover:text-text-primary">
          <svg width="26" height="26" viewBox="0 0 26 26" aria-hidden="true">
            <rect x="1" y="1" width="24" height="24" fill="none" stroke="#EDEDED" stroke-width="2" />
            <path d="M1 13h24M13 1v24" stroke="#EDEDED" stroke-width="2" />
            <rect x="14" y="14" width="10" height="10" fill="#A8FF00" />
          </svg>
          <span>estate</span>
        </RouterLink>
        <nav aria-label="Primary" class="hidden grow items-stretch gap-8 md:flex">
          <RouterLink
            v-for="p in pages"
            :key="p.to"
            :to="p.to"
            :aria-current="current(p.names)"
            class="-mb-px flex items-center gap-1.5 border-b-2 border-transparent pt-0.5 text-nav no-underline hover:text-white aria-[current=page]:border-accent aria-[current=page]:text-white"
          >
            {{ p.label }}<span v-if="p.to === '/alerts' && firing > 0" class="font-semibold text-warn">{{ firing }}</span>
          </RouterLink>
        </nav>
        <DropdownMenuRoot>
          <DropdownMenuTrigger
            aria-label="Account"
            class="ml-auto flex size-10 cursor-pointer items-center justify-center self-center rounded-full border border-control bg-transparent text-nav transition-colors hover:border-accent hover:text-accent"
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <circle cx="12" cy="8" r="4" />
              <path d="M4 21c1.5-4 4.5-6 8-6s6.5 2 8 6" />
            </svg>
          </DropdownMenuTrigger>
          <DropdownMenuPortal>
            <DropdownMenuContent align="end" :side-offset="8" class="z-10 min-w-52 rounded-md border border-line bg-surface-dark p-3 font-sans text-text-primary">
              <DropdownMenuLabel v-if="session.data.value" class="mb-2 text-[15px] text-text-muted">
                Signed in as {{ session.data.value.name }}
              </DropdownMenuLabel>
              <DropdownMenuItem
                class="flex min-h-11 cursor-pointer items-center justify-center rounded-md border border-control font-medium outline-none data-highlighted:border-accent data-highlighted:text-accent"
                @select="signOut?.requestSubmit()"
              >
                Sign out
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenuPortal>
        </DropdownMenuRoot>
        <form ref="signOut" method="post" action="/auth/logout" class="hidden" aria-hidden="true" />
      </div>
    </header>

    <main class="box-border w-full grow px-6 pb-24 md:pb-16 lg:px-10">
      <RouterView />
    </main>

    <nav aria-label="Primary, tab bar" class="fixed inset-x-0 bottom-0 grid auto-cols-fr grid-flow-col border-t border-line bg-surface-dark md:hidden">
      <RouterLink
        v-for="p in pages"
        :key="p.to"
        :to="p.to"
        :aria-current="current(p.names)"
        class="-mt-px flex min-h-13 items-center justify-center gap-1 border-t-2 border-transparent text-sm text-nav no-underline aria-[current=page]:border-accent aria-[current=page]:text-white"
      >
        {{ p.label }}<span v-if="p.to === '/alerts' && firing > 0" class="font-semibold text-warn">{{ firing }}</span>
      </RouterLink>
    </nav>
  </div>
</template>
