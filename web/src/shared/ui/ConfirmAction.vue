<script setup lang="ts">
import {
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogOverlay,
  AlertDialogPortal,
  AlertDialogRoot,
  AlertDialogTitle,
  AlertDialogTrigger,
} from 'reka-ui'

/** A button that asks before it opens a workflow somewhere else, so a misclick does nothing. */
defineProps<{ label: string; title: string; text: string; action: string; href: string }>()

const button =
  'inline-flex min-h-10 cursor-pointer items-center justify-center rounded-md border border-control bg-transparent px-4 font-sans text-[15px] text-text-primary no-underline transition-colors hover:border-accent hover:text-accent max-md:min-h-11'
</script>

<template>
  <AlertDialogRoot>
    <AlertDialogTrigger :class="button">{{ label }}</AlertDialogTrigger>
    <AlertDialogPortal>
      <AlertDialogOverlay class="fixed inset-0 z-20 bg-black/60" />
      <AlertDialogContent
        class="fixed top-1/2 left-1/2 z-30 w-[min(28rem,calc(100vw-2rem))] -translate-1/2 rounded-md border border-line bg-surface-dark p-6 font-sans text-text-primary shadow-2xl"
      >
        <AlertDialogTitle class="m-0 text-xl font-semibold">{{ title }}</AlertDialogTitle>
        <AlertDialogDescription class="m-0 mt-2 text-text-muted">{{ text }}</AlertDialogDescription>
        <div class="mt-6 flex flex-wrap justify-end gap-2">
          <AlertDialogCancel :class="button">Cancel</AlertDialogCancel>
          <AlertDialogAction as-child>
            <a
              :href="href"
              target="_blank"
              rel="noopener"
              :class="[button, 'border-accent bg-accent text-black hover:text-black']"
              >{{ action }}<span class="sr-only"> (opens in a new tab)</span></a
            >
          </AlertDialogAction>
        </div>
      </AlertDialogContent>
    </AlertDialogPortal>
  </AlertDialogRoot>
</template>
