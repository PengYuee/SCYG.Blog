<script setup lang="ts">
import { CheckCircleIcon, ExclamationCircleIcon, ExclamationTriangleIcon, InformationCircleIcon, XMarkIcon } from "@heroicons/vue/24/outline"
import type { Component } from "vue"
import type { ToastTone } from "@/types/common"
import { useUiStore } from "@/stores/ui"

/** 全局 UI 状态。 */
const uiStore = useUiStore()

/** Toast 图标映射。 */
const TONE_ICONS: Record<ToastTone, Component> = {
  success: CheckCircleIcon,
  warning: ExclamationTriangleIcon,
  error: ExclamationCircleIcon,
  info: InformationCircleIcon,
}

/** Toast 色彩映射。 */
const TONE_CLASSES: Record<ToastTone, string> = {
  success: "border-success/20 text-success",
  warning: "border-warning/20 text-warning",
  error: "border-error/20 text-error",
  info: "border-accent/20 text-accent",
}
</script>

<template>
  <div class="pointer-events-none fixed inset-x-4 top-4 z-[70] flex flex-col items-end gap-3 sm:left-auto sm:w-96" aria-live="polite" aria-label="系统通知">
    <TransitionGroup enter-active-class="transition duration-200 ease-out" enter-from-class="translate-y-2 opacity-0" enter-to-class="translate-y-0 opacity-100" leave-active-class="transition duration-150 ease-in" leave-from-class="opacity-100" leave-to-class="opacity-0">
      <article v-for="toast in uiStore.toasts" :key="toast.id" class="pointer-events-auto flex w-full items-start gap-3 rounded-xl border bg-surface p-4 shadow-dialog" :class="TONE_CLASSES[toast.tone]" :role="toast.tone === 'error' ? 'alert' : 'status'">
        <component :is="TONE_ICONS[toast.tone]" class="mt-0.5 size-5 shrink-0" aria-hidden="true" />
        <div class="min-w-0 flex-1">
          <p class="text-sm font-semibold text-text-primary">{{ toast.title }}</p>
          <p v-if="toast.description" class="mt-1 text-sm leading-5 text-text-secondary">{{ toast.description }}</p>
        </div>
        <button type="button" class="inline-flex size-11 shrink-0 items-center justify-center rounded-lg text-text-secondary transition hover:bg-surface-hover hover:text-text-primary" aria-label="关闭通知" @click="uiStore.dismissToast(toast.id)">
          <XMarkIcon class="size-5" aria-hidden="true" />
        </button>
      </article>
    </TransitionGroup>
  </div>
</template>