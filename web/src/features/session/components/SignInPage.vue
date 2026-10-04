<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { signInHref } from '../domain/signIn'
import AccessPanel from './AccessPanel.vue'

const route = useRoute()
const href = computed(() => signInHref(route.query.next))
// The server sends a sign-in that did not complete back here; why is in its log, not in the address.
const failed = computed(() => route.query.failed !== undefined)
</script>

<template>
  <AccessPanel>
    <div class="lead">
      <h1>Sign in</h1>
      <p>The estate's delivery dashboard, for admins. You sign in with your jorisjonkers.dev account.</p>
    </div>
    <p v-if="failed" role="alert" class="failed">Signing in did not complete. Try again.</p>
    <a class="action" :href="href">Continue with jorisjonkers.dev</a>
    <p class="small">You return here once auth has signed you in. Two-factor, if your account has it, is asked there.</p>
    <div class="links">
      <a href="https://auth.jorisjonkers.dev/forgot-password">Forgot your password?</a>
      <a href="https://jorisjonkers.dev">jorisjonkers.dev</a>
    </div>
  </AccessPanel>
</template>

<style scoped>
.failed {
  color: #ff9e94;
}
</style>
