<script setup lang="ts">
import '@fontsource/barlow-semi-condensed/latin-400.css'
import '@fontsource/barlow-semi-condensed/latin-600.css'
import '@fontsource/barlow-semi-condensed/latin-700.css'

defineProps<{ wide?: boolean }>()
</script>

<template>
  <div class="page">
    <main class="panel" :class="{ wide }">
      <div class="brand">
        <svg width="34" height="34" viewBox="0 0 26 26" aria-hidden="true">
          <rect x="1" y="1" width="24" height="24" fill="none" stroke="#EDEDED" stroke-width="2" />
          <path d="M1 13h24M13 1v24" stroke="#EDEDED" stroke-width="2" />
          <rect x="14" y="14" width="10" height="10" fill="#A8FF00" />
        </svg>
        <span class="name">estate</span>
      </div>
      <slot />
    </main>
  </div>
</template>

<style scoped>
/* One centred surface on a deeper page, a hairline edge and a faint accent glow, as auth draws its sign-in. */
.page {
  display: flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  min-height: 100vh;
  padding: 64px 32px;
  font-family: 'Barlow Semi Condensed', system-ui, sans-serif;
  font-size: 17px;
  line-height: 1.5;
  color: #ededed;
  background: #141414;
}
.panel {
  display: flex;
  flex-direction: column;
  gap: 18px;
  box-sizing: border-box;
  width: 100%;
  max-width: 440px;
  padding: 44px 48px;
  background: #1e1e1e;
  border: 1px solid #333333;
  border-radius: 2px;
  box-shadow:
    0 0 24px rgba(168, 255, 0, 0.06),
    0 0 72px rgba(168, 255, 0, 0.03);
}
.panel.wide {
  max-width: 480px;
}
.brand {
  display: flex;
  gap: 12px;
  align-items: center;
}
.name {
  font-size: 22px;
  font-weight: 600;
  letter-spacing: 0.04em;
}
.panel :deep(.lead) {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.panel :deep(h1) {
  margin: 0;
  font-size: 30px;
  font-weight: 600;
}
.panel :deep(p) {
  margin: 0;
  color: #a3a3a3;
}
.panel :deep(.small) {
  font-size: 15px;
}
.panel :deep(a) {
  color: #549aff;
}
.panel :deep(a:hover) {
  color: #8ab9ff;
}
/* The one action. White on this blue reaches AA only as large bold text, so it is set as that. */
.panel :deep(.action) {
  display: flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  width: 100%;
  min-height: 48px;
  padding: 0;
  font: inherit;
  font-size: 19px;
  font-weight: 700;
  color: #ffffff;
  text-decoration: none;
  cursor: pointer;
  background: #3387fa;
  border: 0;
  border-radius: 2px;
  outline-offset: 2px;
}
.panel :deep(.action:hover) {
  color: #ffffff;
}
.panel :deep(.links) {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  justify-content: space-between;
  padding-top: 14px;
  font-size: 15px;
  border-top: 1px solid #333333;
}
@media (max-width: 480px) {
  .page {
    padding: 20px 16px;
  }
  .panel,
  .panel.wide {
    max-width: 100%;
    padding: 36px 24px;
  }
  .panel :deep(h1) {
    font-size: 26px;
  }
}
</style>
