<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { signInHref } from '../domain/signIn'
import AccessPanel from './AccessPanel.vue'

const route = useRoute()
const href = computed(() => signInHref(route.query.next))
// The reason for a failed sign-in is in the server log.
const failed = computed(() => route.query.failed !== undefined)
</script>

<template>
  <AccessPanel>
    <div class="flex flex-col gap-1.5">
      <h1 class="m-0 text-[26px] font-semibold sm:text-3xl">Sign in</h1>
      <p class="m-0 text-text-muted">
        The estate's delivery dashboard, for admins. Sign in with your jorisjonkers.dev account.
      </p>
    </div>
    <p v-if="failed" role="alert" class="m-0 text-crit">Sign-in did not finish. Try again.</p>
    <a
      class="action flex min-h-12 items-center justify-center rounded-md bg-accent text-lg font-semibold text-[#1c1c1c] no-underline hover:bg-accent/85 hover:text-[#1c1c1c]"
      :href="href"
      >Continue with jorisjonkers.dev</a
    >
    <p class="m-0 text-[15px] text-text-muted">
      You come back here after you sign in. If your account has two-factor, auth asks for it there.
    </p>
    <div class="links flex flex-wrap justify-between gap-3 border-t border-line pt-3.5 text-[15px]">
      <a href="https://auth.jorisjonkers.dev/forgot-password">Forgot your password?</a>
      <a href="https://jorisjonkers.dev">jorisjonkers.dev</a>
    </div>
  </AccessPanel>
</template>
