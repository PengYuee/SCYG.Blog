<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue"
import { useRoute } from "vue-router"
import type { AgentResumeRequest } from "@/request/generated"
import { useApiServices } from "@/request/api-services"
import { useAuthSession } from "@/services/auth-session"
import { authorTabSubject, onAuthorSessionChange } from "@/services/ai-session-state"
import { ensureAnonymousSession, onAnonymousSessionChange } from "@/services/anonymous-ai-session"
import { createAiRun, RUN_STATUS_LABELS, isTerminalRun } from "@/services/ai-run"
import AiError from "@/components/ai/AiError.vue"
import AiResult from "@/components/ai/AiResult.vue"
import AiInteraction from "@/components/ai/AiInteraction.vue"
const props = defineProps<{ anonymous: boolean }>()
const route = useRoute()
const api = useApiServices().ai
const author = props.anonymous ? undefined : useAuthSession()
const bootError = ref<unknown>(null)
// A component instance owns exactly one lifecycle, including on same-route param changes.
let lifecycle: AiRunController | undefined
import type { AiRunController } from "@/services/ai-run"
const state = ref<AiRunController["state"] | null>(null)
let bootGeneration = 0
let bootController: AbortController | undefined
const unsubscribe = props.anonymous ? onAnonymousSessionChange(invalidate) : onAuthorSessionChange(invalidate)
function invalidate(): void {
  bootGeneration += 1
  bootController?.abort()
  lifecycle?.stop()
  state.value = null
  bootError.value = new Error(props.anonymous ? "匿名身份已失效，不能使用新凭证访问原任务。" : "作者会话已改变，旧任务已停止本地订阅。")
}
watch(() => route.params["runId"], async (value) => {
  const own = ++bootGeneration
  bootController?.abort()
  bootController = new AbortController()
  lifecycle?.stop()
  state.value = null
  bootError.value = null
  const runId = typeof value === "string" ? value : ""
  if (!/^[A-Za-z0-9._~-]{1,128}$/.test(runId) || runId === "." || runId === "..") { bootError.value = new Error("无效 Run 标识"); return }
  try {
    const subject = props.anonymous ? (await ensureAnonymousSession(api, false, bootController.signal)).accessToken : authorTabSubject(author?.accessToken() ?? "")
    if (own !== bootGeneration) return
    const key = `scyg.ai.${props.anonymous ? 'anonymous' : 'author'}.${subject}.run.${runId}`
    lifecycle = createAiRun(api, props.anonymous ? "anonymous" : "author", runId, key)
    state.value = lifecycle.state
    await lifecycle.connect()
  } catch (error) { if (own === bootGeneration) bootError.value = error }
}, { immediate: true })
onBeforeUnmount(() => { bootGeneration += 1; bootController?.abort(); lifecycle?.stop(); unsubscribe() })
const locked = computed(() => state.value === null || state.value.busy || state.value.now < state.value.waitUntil)
const ended = computed(() => state.value?.run !== null && state.value?.run !== undefined && isTerminalRun(state.value.run))
async function refresh(): Promise<void> {
  const current = lifecycle
  const own = bootGeneration
  try { await current?.refresh() }
  catch (error) {
    if (current !== undefined && own === bootGeneration && lifecycle === current && state.value === current.state) current.state.error = error
  }
}
async function cancel(): Promise<void> {
  if (locked.value || !window.confirm("确定取消这个 Run？已开始的工作无法撤回。")) return
  await lifecycle?.mutate("cancel")
}
function resume(body: AgentResumeRequest): void { void lifecycle?.mutate("resume", body) }
function interactionDraft(value: unknown): void {
  const interactionId = state.value?.run?.pendingInteraction?.interactionId
  if (state.value !== null && interactionId !== undefined) { state.value.interactionDrafts[interactionId] = value; lifecycle?.persist() }
}
</script>
<template>
  <div class="ai-workspace">
    <header><h1 v-if="!anonymous">AI 任务</h1><RouterLink :to="anonymous ? '/ai/search' : '/author/ai'">返回{{ anonymous ? '公共搜索' : '工作台' }}</RouterLink><p class="break-all">Run：{{ route.params['runId'] }}</p></header>
    <AiError :error="bootError" />
    <template v-if="state">
      <section class="ai-panel" aria-label="运行状态" :aria-busy="state.busy">
        <h2>{{ state.run ? RUN_STATUS_LABELS[state.run.status] : '查询任务中' }}</h2>
        <p role="status" aria-live="polite">{{ state.connection }}</p>
        <p v-if="state.run">能力：{{ state.run.capability }} · 最近更新：{{ state.run.updatedAt }}</p>
        <p v-if="state.now < state.waitUntil" role="status">请等待 {{ Math.ceil((state.waitUntil-state.now)/1000) }} 秒后手动重试。</p>
        <div class="flex flex-wrap gap-4"><button type="button" :disabled="locked" @click="refresh">查询最新状态</button><button v-if="!ended" type="button" :disabled="locked" @click="lifecycle?.connect()">手动重连</button><button v-if="!ended" type="button" :disabled="locked" @click="lifecycle?.connect(true)">无游标重新连接</button><button v-if="!ended" type="button" :disabled="locked" @click="cancel">取消任务</button></div>
        <template v-if="state.pendingOperation"><p>上一{{ state.pendingOperation === 'resume' ? '交互回应' : '取消' }}操作结果尚未确认。重试保持原始操作标识和内容。</p><button type="button" :disabled="locked" @click="lifecycle?.retryPending()">重试原操作</button></template>
        <p>离开页面仅停止本地订阅，不会取消服务端任务。</p>
        <AiError :error="state.error" />
      </section>
      <AiInteraction v-if="!anonymous && state.run?.pendingInteraction" :key="state.run.pendingInteraction.interactionId" :interaction="state.run.pendingInteraction" :draft="state.interactionDrafts[state.run.pendingInteraction.interactionId]" :disabled="locked" @submit="resume" @draft="interactionDraft" />
      <AiResult v-if="state.run" :run="state.run" :anonymous="anonymous" :original="state.original" />
      <section class="ai-panel" aria-label="活动记录"><h2>实际活动</h2><p v-if="state.activities.length === 0">尚无活动通知。运行状态以上方快照为准。</p><ol class="max-h-64 overflow-auto"><li v-for="(activity,index) in state.activities" :key="index">{{ activity }}</li></ol></section>
    </template>
  </div>
</template>
