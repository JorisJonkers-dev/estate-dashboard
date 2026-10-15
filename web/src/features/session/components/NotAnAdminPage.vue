<script setup lang="ts">
import AccessPanel from './AccessPanel.vue'
</script>

<template>
  <AccessPanel wide>
    <div class="lead">
      <h1>This dashboard is for admins</h1>
      <p class="said">
        You're signed in, but your account doesn't have the admin role. Ask an admin to grant it in auth, then sign in
        again.
      </p>
    </div>
    <!-- A form, not a link, so other pages can't sign the account out. -->
    <form method="post" action="/auth/switch">
      <button type="submit" class="action">Sign in as someone else</button>
    </form>
    <div class="links">
      <a href="https://jorisjonkers.dev">Back to jorisjonkers.dev</a>
    </div>
  </AccessPanel>
</template>

<style scoped>
.said {
  color: #c8c8c8;
}
form {
  margin: 0;
}
</style>
