<script setup lang="ts">
import { computed } from "vue"
import { HttpRequestError } from "@/request/http-error"
const props = defineProps<{ error: unknown }>()
const feedback = computed(() => {
  const error = props.error
  if (error instanceof HttpRequestError) return { message: error.detail ?? error.message, status: error.status, code: error.problem?.code ?? error.code, requestId: error.requestId }
  return { message: error instanceof Error ? error.message : "请求失败，请检查最新状态后重试", status: undefined, code: undefined, requestId: undefined }
})
</script>
<template>
  <section v-if="error" role="alert" class="rounded-lg border border-error bg-error-soft p-4 text-error">
    <p>{{ feedback.message }}</p>
    <p v-if="feedback.status || feedback.code" class="text-sm">{{ feedback.status }} · {{ feedback.code }}</p>
    <p v-if="feedback.requestId" class="break-all text-sm">请求标识：{{ feedback.requestId }}</p>
  </section>
</template>
