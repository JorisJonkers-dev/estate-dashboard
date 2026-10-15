<script setup lang="ts">
import { computed } from 'vue'
import HeadBand from '../../../shared/ui/HeadBand.vue'
import ProjectsList from '../../overview/components/ProjectsList.vue'
import { useOverview } from '../../overview/composables/useOverview'

const { isLoading, projects } = useOverview()
const onEstate = computed(() => projects.value.filter((p) => p.path === 'estate').length)
</script>

<template>
  <div class="flex flex-col gap-8 md:gap-10">
    <HeadBand eyebrow="Production" title="Projects">{{ projects.length }} projects · {{ onEstate }} on the estate path</HeadBand>
    <ProjectsList :projects="projects" :loading="isLoading" />
  </div>
</template>
