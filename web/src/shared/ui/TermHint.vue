<script setup lang="ts">
import { TooltipArrow, TooltipContent, TooltipPortal, TooltipProvider, TooltipRoot, TooltipTrigger } from 'reka-ui'

/** Explains what it wraps on hover and focus; a `term` is plain text, so it gets a tab stop. */
defineProps<{ text?: string | undefined; term?: boolean }>()
</script>

<template>
  <slot v-if="!text" />
  <TooltipProvider v-else :delay-duration="250">
    <TooltipRoot>
      <TooltipTrigger as-child>
        <span v-if="term" tabindex="0" class="cursor-help underline decoration-dim decoration-dotted underline-offset-4"
          ><slot
        /></span>
        <slot v-else />
      </TooltipTrigger>
      <TooltipPortal>
        <TooltipContent
          :side-offset="6"
          class="z-40 max-w-72 rounded-md border border-line bg-[#2a2a2a] px-3 py-2 font-sans text-sm leading-snug font-normal tracking-normal text-text-primary normal-case shadow-lg"
        >
          {{ text }}
          <TooltipArrow class="fill-[#2a2a2a]" />
        </TooltipContent>
      </TooltipPortal>
    </TooltipRoot>
  </TooltipProvider>
</template>
