<script setup lang="ts">
/** A seam between two bands: leaning dashes gathering into a line that fades out. Decorative. */
defineProps<{ mirrored?: boolean }>()
</script>

<template>
  <div aria-hidden="true" class="band-rule" :class="{ mirrored }">
    <span class="ticks" />
    <span class="line" />
  </div>
</template>

<style scoped>
.band-rule {
  display: flex;
  height: 2px;
  margin-block: -1px;
}
.mirrored {
  transform: scaleX(-1);
}
.ticks {
  flex: 0 0 4.9rem;
  background-image: linear-gradient(
    112deg,
    var(--color-action) 0 3px,
    transparent 3px 8px,
    var(--color-action) 8px 12px,
    transparent 12px 17px,
    var(--color-action) 17px 22px,
    transparent 22px 26px,
    var(--color-action) 26px 32px,
    transparent 32px 36px,
    var(--color-action) 36px 43px,
    transparent 43px 46px,
    var(--color-action) 46px 54px,
    transparent 54px 57px,
    var(--color-action) 57px 66px,
    transparent 66px 68px,
    var(--color-action) 68px 100%
  );
}
.line {
  flex: 1 1 auto;
  background: linear-gradient(90deg, var(--color-action), transparent);
}
</style>
