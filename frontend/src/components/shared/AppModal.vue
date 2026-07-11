<script setup lang="ts">
import { Dialog, DialogPanel, DialogTitle, TransitionChild, TransitionRoot } from "@headlessui/vue"
import { XMarkIcon } from "@heroicons/vue/24/outline"

/** 弹窗输入属性。 */
type Props = {
  /** 是否打开弹窗。 */
  readonly open: boolean
  /** 弹窗标题。 */
  readonly title: string
  /** 可选说明文本。 */
  readonly description?: string
}

defineProps<Props>()

/** 弹窗关闭事件。 */
defineEmits<{ close: [] }>()
</script>

<template>
  <TransitionRoot appear :show="open">
    <Dialog as="div" class="relative z-50" @close="$emit('close')">
      <TransitionChild
        enter="ease-out duration-200"
        enter-from="opacity-0"
        enter-to="opacity-100"
        leave="ease-in duration-150"
        leave-from="opacity-100"
        leave-to="opacity-0"
      >
        <div class="fixed inset-0 bg-overlay" aria-hidden="true" />
      </TransitionChild>

      <div class="fixed inset-0 overflow-y-auto p-4 sm:p-6">
        <div class="flex min-h-full items-center justify-center">
          <TransitionChild
            enter="ease-out duration-200"
            enter-from="opacity-0 translate-y-2 scale-[0.98]"
            enter-to="opacity-100 translate-y-0 scale-100"
            leave="ease-in duration-150"
            leave-from="opacity-100 translate-y-0 scale-100"
            leave-to="opacity-0 translate-y-2 scale-[0.98]"
          >
            <DialogPanel class="w-full max-w-lg rounded-2xl border border-border bg-surface p-6 shadow-dialog">
              <div class="flex items-start justify-between gap-4">
                <div>
                  <DialogTitle class="text-lg font-semibold tracking-tight text-text-primary">{{ title }}</DialogTitle>
                  <p v-if="description" class="mt-1 text-sm leading-6 text-text-secondary">{{ description }}</p>
                </div>
                <button type="button" class="inline-flex size-11 shrink-0 items-center justify-center rounded-lg text-text-secondary transition hover:bg-surface-hover hover:text-text-primary" aria-label="关闭弹窗" @click="$emit('close')">
                  <XMarkIcon class="size-5" aria-hidden="true" />
                </button>
              </div>
              <div class="mt-6"><slot /></div>
              <div v-if="$slots['footer']" class="mt-6 flex flex-wrap justify-end gap-3 border-t border-border-subtle pt-4"><slot name="footer" /></div>
            </DialogPanel>
          </TransitionChild>
        </div>
      </div>
    </Dialog>
  </TransitionRoot>
</template>