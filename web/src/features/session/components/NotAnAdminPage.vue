<script setup lang="ts">
import AccessPanel from './AccessPanel.vue'
</script>

<template>
  <AccessPanel wide>
    <div class="flex flex-col gap-1.5">
      <h1 class="m-0 text-[26px] font-semibold sm:text-3xl">This dashboard is for admins</h1>
      <p class="m-0 text-text-muted">
        You're signed in, but your account doesn't have the admin role. Ask an admin to grant it in auth, then sign in
        again.
      </p>
    </div>
    <!-- A form, not a link, so other pages can't sign the account out. -->
    <form method="post" action="/auth/switch" class="m-0">
      <button
        type="submit"
        class="action flex min-h-12 w-full cursor-pointer items-center justify-center rounded-md border-0 bg-accent font-sans text-lg font-semibold text-[#1c1c1c] hover:bg-accent/85"
      >
        Sign in as someone else
      </button>
    </form>
    <div class="links flex flex-wrap justify-between gap-3 border-t border-line pt-3.5 text-[15px]">
      <a href="https://jorisjonkers.dev">Back to jorisjonkers.dev</a>
    </div>
  </AccessPanel>
</template>
