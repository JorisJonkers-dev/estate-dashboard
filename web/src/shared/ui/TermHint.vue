<script setup lang="ts">
import { TooltipArrow, TooltipContent, TooltipPortal, TooltipProvider, TooltipRoot, TooltipTrigger } from 'reka-ui'

/** Explains what it wraps, on hover and on keyboard focus. */
defineProps<{ text: string }>()
</script>

<template>
  <TooltipProvider :delay-duration="250">
    <TooltipRoot>
      <TooltipTrigger as-child><slot /></TooltipTrigger>
      <TooltipPortal>
        <TooltipContent :side-offset="6" class="z-40 max-w-72 rounded-md border border-line bg-[#2a2a2a] px-3 py-2 font-sans text-sm leading-snug text-text-primary shadow-lg">
          {{ text }}
          <TooltipArrow class="fill-[#2a2a2a]" />
        </TooltipContent>
      </TooltipPortal>
    </TooltipRoot>
  </TooltipProvider>
</template>
