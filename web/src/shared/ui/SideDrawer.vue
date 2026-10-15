<script setup lang="ts">
import { DialogClose, DialogContent, DialogDescription, DialogOverlay, DialogPortal, DialogRoot, DialogTitle } from 'reka-ui'

/** Side panel for logs and notes. Escape closes it. */
const open = defineModel<boolean>('open', { required: true })
defineProps<{ title: string; subtitle: string }>()
</script>

<template>
  <DialogRoot v-model:open="open">
    <DialogPortal>
      <DialogOverlay class="fixed inset-0 z-20 bg-black/50" />
      <DialogContent
        class="fixed inset-y-0 right-0 z-30 flex w-full max-w-4xl flex-col border-l border-line bg-surface-dark font-sans text-text-primary shadow-2xl outline-none"
      >
        <header class="flex items-start justify-between gap-4 border-b border-line bg-band px-6 py-4">
          <div class="min-w-0">
            <DialogTitle class="m-0 truncate text-xl font-semibold">{{ title }}</DialogTitle>
            <DialogDescription class="m-0 mt-0.5 truncate text-sm text-text-muted">{{ subtitle }}</DialogDescription>
          </div>
          <DialogClose aria-label="Close" class="flex size-9 shrink-0 cursor-pointer items-center justify-center rounded-md border border-control bg-transparent text-nav transition-colors hover:border-accent hover:text-accent">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18" /></svg>
          </DialogClose>
        </header>
        <div class="min-h-0 grow overflow-auto"><slot /></div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>
