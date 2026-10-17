<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from "vue"
import { z } from "zod"
import { useRouter } from "vue-router"
import type { AgentRun } from "@/request/generated"
import { useApiServices } from "@/request/api-services"
import { useAuthSession } from "@/services/auth-session"
import { ensureAnonymousSession, onAnonymousSessionChange, restoreAnonymousSession } from "@/services/anonymous-ai-session"
import { authorTabSubject, onAuthorSessionChange, readTabState, saveTabState } from "@/services/ai-session-state"
import { HttpRequestError } from "@/request/http-error"
import AiArticlePicker from "@/components/ai/AiArticlePicker.vue"
import AiError from "@/components/ai/AiError.vue"
const props = defineProps<{ anonymous: boolean }>()
const router = useRouter()
const api = useApiServices().ai
const author = props.anonymous ? undefined : useAuthSession()
const auth = props.anonymous ? "anonymous" : "author"
const controller = new AbortController()
let generation = 0
const limits = reactive({ queryMaxChars: 2000, maxResults: 5 })
const currentAnonymous = props.anonymous ? restoreAnonymousSession() : undefined
if (currentAnonymous !== undefined) { limits.queryMaxChars = currentAnonymous.queryMaxChars; limits.maxResults = currentAnonymous.maxResults }
const subject = props.anonymous ? currentAnonymous?.accessToken : authorTabSubject(author?.accessToken() ?? "")
let storageKey = props.anonymous ? `scyg.ai.anonymous.${subject ?? 'new'}.workspace` : `scyg.ai.author.${subject}.workspace`
type FormDraft = { capability: AgentRun["capability"]; query: string; maxResults: number; topic: string; requirements: string; content: string; message: string; requestRequirements: boolean; referenceMode: "manual" | "search"; referenceQuery: string; referenceIds: number[] }
const defaults: FormDraft = { capability: "search", query: "", maxResults: 5, topic: "", requirements: "", content: "", message: "", requestRequirements: false, referenceMode: "manual", referenceQuery: "", referenceIds: [] }
const workspaceModel = z.object({
  form: z.object({ capability: z.enum(["search", "write", "polish", "chat"]), query: z.string(), maxResults: z.number().int(), topic: z.string(), requirements: z.string(), content: z.string(), message: z.string(), requestRequirements: z.boolean(), referenceMode: z.enum(["manual", "search"]), referenceQuery: z.string(), referenceIds: z.array(z.number().int().positive().max(Number.MAX_SAFE_INTEGER)).max(20) }).optional(),
  lastRun: z.string().regex(/^[A-Za-z0-9._~-]{1,128}$/).optional(),
  waitUntil: z.number().finite().nonnegative().optional(),
  pending: z.object({ key: z.uuid(), input: z.record(z.string(), z.unknown()), capability: z.enum(["search", "write", "polish", "chat"]), createdAt: z.number().finite().nonnegative() }).optional(),
})
const decodedWorkspace = workspaceModel.safeParse(readTabState<unknown>(storageKey, {}))
const stored = decodedWorkspace.success ? decodedWorkspace.data : {}
const form = reactive<FormDraft>({ ...defaults, ...stored.form })
if (props.anonymous) form.capability = "search"
const pending = ref(stored.pending)
const invalidated = ref(false)
const pendingExpired = computed(() => pending.value !== undefined && now.value - pending.value.createdAt >= 86400000)
const busy = ref(false)
const error = ref<unknown>(null)
const waitUntil = ref(stored.waitUntil ?? 0)
const now = ref(Date.now())
const lastRun = ref<string | undefined>(stored.lastRun)
const interval = window.setInterval(() => { now.value = Date.now() }, 250)
function persist(): void { if (!invalidated.value) saveTabState(storageKey, { form, pending: pending.value, lastRun: lastRun.value, waitUntil: waitUntil.value }) }
watch(form, persist, { deep: true, flush: "sync" })
const unsubscribe = props.anonymous ? onAnonymousSessionChange(invalidate) : onAuthorSessionChange(invalidate)
function invalidate(): void {
  generation += 1
  controller.abort()
  invalidated.value = true
  pending.value = undefined
  lastRun.value = undefined
  waitUntil.value = 0
  Object.assign(form, defaults, { referenceIds: [] })
  busy.value = false
  error.value = new Error(props.anonymous ? "匿名会话已失效，旧任务不能使用新凭证恢复。" : "作者会话已变更，请重新进入工作台。")
}
onBeforeUnmount(() => { generation += 1; controller.abort(); clearInterval(interval); unsubscribe() })
const disabled = computed(() => busy.value || now.value < waitUntil.value || controller.signal.aborted)
const validation = computed(() => {
  switch (form.capability) {
    case "search": return form.query.trim().length > 0 && form.query.length <= (props.anonymous ? limits.queryMaxChars : 16000) && Number.isInteger(form.maxResults) && form.maxResults >= 1 && form.maxResults <= (props.anonymous ? limits.maxResults : 20)
    case "write": return form.topic.trim().length > 0 && form.topic.length <= 2000 && form.requirements.length <= 16000 && !(form.requestRequirements && form.requirements.trim()) && (form.referenceMode === "manual" ? form.referenceIds.length <= 20 : form.referenceQuery.trim().length > 0 && form.referenceQuery.length <= 500)
    case "polish": return form.content.trim().length > 0 && form.content.length <= 100000 && form.requirements.length <= 16000
    case "chat": return form.message.trim().length > 0 && form.message.length <= 16000
  }
})
function abandonExpiredCreate(): void {
  if (disabled.value || !pendingExpired.value) return
  if (!window.confirm("旧操作可能已经执行并产生费用。放弃只解除本页的未知操作记录，不会取消远端任务；再次提交会使用新的操作标识，可能重复执行并再次产生费用。确认放弃未知操作并开始新操作？")) return
  pending.value = undefined
  error.value = null
  persist()
}
async function submit(): Promise<void> {
  if (disabled.value || pendingExpired.value || (!pending.value && !validation.value)) return
  busy.value = true
  error.value = null
  const own = generation
  try {
    if (props.anonymous) {
      const session = await ensureAnonymousSession(api, pending.value === undefined, controller.signal)
      if (own !== generation) return
      limits.queryMaxChars = session.queryMaxChars
      limits.maxResults = session.maxResults
      storageKey = `scyg.ai.anonymous.${session.accessToken}.workspace`
      if (!validation.value && pending.value === undefined) throw new Error("输入超过当前公共搜索限制，请修正后提交。")
    }
    if (pending.value === undefined) {
      let input: Record<string, unknown>
      switch (form.capability) {
        case "search": input = { query: form.query, max_results: form.maxResults }; break
        case "write": input = { topic: form.topic, requirements: form.requirements, ...(form.requestRequirements ? { request_requirements: true } : {}), ...(form.referenceMode === "search" ? { reference_query: form.referenceQuery.trim() } : { reference_article_ids: [...form.referenceIds] }) }; break
        case "polish": input = { content: form.content, requirements: form.requirements }; break
        case "chat": input = { message: form.message }; break
      }
      pending.value = { key: crypto.randomUUID(), input, capability: form.capability, createdAt: Date.now() }
      persist()
    }
    if (Date.now() - pending.value.createdAt >= 86400000) throw new Error("未知操作超过幂等窗口，不能自动重放；旧操作可能已执行，请查询已有 Run，或明确确认放弃后再开始新操作。")
    const run = await api.create(pending.value.capability, pending.value.input, pending.value.key, auth, controller.signal)
    if (own !== generation) return
    const runKey = `${storageKey.replace(/\.workspace$/, '')}.run.${run.runId}`
    saveTabState(runKey, { interactionDrafts: {}, original: pending.value.capability === "polish" ? pending.value.input["content"] : undefined })
    lastRun.value = run.runId
    pending.value = undefined
    persist()
    await router.push(`${props.anonymous ? '/ai/search' : '/author/ai'}/runs/${run.runId}`)
  } catch (failure) {
    if (own !== generation) return
    error.value = failure
    if (failure instanceof HttpRequestError) {
      if (failure.status === 429 && failure.retryAfterSeconds !== undefined) waitUntil.value = Date.now() + failure.retryAfterSeconds * 1000
      if (failure.status !== undefined && failure.status < 500 && failure.status !== 429) { pending.value = undefined; persist() }
    }
    persist()
  } finally { if (own === generation) busy.value = false }
}
</script>
<template>
  <div class="ai-workspace">
    <header><h1 v-if="!anonymous">AI 工作台</h1><p>{{ anonymous ? '仅检索公开文章；匿名身份与作者登录独立。' : '搜索、写作、润色与问答。生成内容不会自动保存或发布。' }}</p></header>
    <p v-if="invalidated" role="alert">{{ anonymous ? '匿名会话已失效，旧任务不能使用新凭证恢复。' : '作者会话已变更，旧内容已清除，请重新进入工作台。' }}</p>
    <form v-else class="ai-panel" :aria-busy="busy" @submit.prevent="submit">
      <fieldset v-if="!anonymous" :disabled="disabled || !!pending"><legend>选择能力</legend><div class="flex gap-4"><label v-for="item in [{value:'search',label:'站内搜索'},{value:'write',label:'写作草稿'},{value:'polish',label:'润色对照'},{value:'chat',label:'独立问答'}]" :key="item.value"><input v-model="form.capability" type="radio" name="capability" :value="item.value" /> {{ item.label }}</label></div></fieldset>
      <fieldset :disabled="disabled || !!pending">
        <template v-if="form.capability === 'search'"><label>搜索问题<textarea v-model="form.query" required rows="6" :maxlength="anonymous ? limits.queryMaxChars : 16000" /></label><label>最多结果数<input v-model.number="form.maxResults" type="number" min="1" :max="anonymous ? limits.maxResults : 20" required /></label><p v-if="anonymous">当前限制：问题最多 {{ limits.queryMaxChars }} 字符，最多 {{ limits.maxResults }} 篇结果。</p></template>
        <template v-else-if="form.capability === 'write'">
          <label>写作主题<input v-model="form.topic" required maxlength="2000" /></label><label>写作要求<textarea v-model="form.requirements" rows="5" maxlength="16000" :disabled="form.requestRequirements" /></label>
          <label><input v-model="form.requestRequirements" type="checkbox" :disabled="!!form.requirements.trim()" /> 稍后补充写作要求（生成前暂停）</label>
          <fieldset><legend>参考来源</legend><label><input v-model="form.referenceMode" type="radio" value="manual" /> 手选参考 ID</label><label><input v-model="form.referenceMode" type="radio" value="search" /> 检索参考，多个匹配时由我选择</label></fieldset>
          <AiArticlePicker v-if="form.referenceMode === 'manual'" v-model="form.referenceIds" :disabled="disabled || !!pending" />
          <template v-else><label>参考搜索词<input v-model="form.referenceQuery" maxlength="500" required /></label><p>零候选不使用参考；唯一候选自动采用；至少两篇时由你多选。选中参考将提供有来源的正文片段。</p></template>
        </template>
        <template v-else-if="form.capability === 'polish'"><label>原文<textarea v-model="form.content" required rows="12" maxlength="100000" /></label><label>润色要求<textarea v-model="form.requirements" rows="4" maxlength="16000" /></label></template>
        <template v-else><label>你的问题<textarea v-model="form.message" required rows="8" maxlength="16000" /></label><p>每次提交都是独立 Run，不继承其他问答的上下文。</p></template>
      </fieldset>
      <p v-if="pendingExpired" role="alert">未知创建操作已超过幂等窗口，不能重试原操作。旧操作可能已执行并产生费用。放弃不会取消远端任务；再次提交可能重复执行并再次产生费用。</p>
      <p v-else-if="pending" role="status">上一创建操作结果尚未确认；重试将保留原始输入和操作标识，不创建新的逻辑操作。</p>
      <button v-if="pendingExpired" type="button" :disabled="disabled" @click="abandonExpiredCreate">放弃未知操作并开始新操作</button>
      <p v-if="now < waitUntil" role="status">服务繁忙，请等待 {{ Math.ceil((waitUntil-now)/1000) }} 秒后手动重试。</p>
      <button type="submit" :disabled="disabled || pendingExpired || (!pending && !validation)">{{ busy ? '提交中…' : pending ? '重试原创建操作' : '开始运行' }}</button>
      <AiError :error="error" />
      <RouterLink v-if="lastRun" :to="`${anonymous ? '/ai/search' : '/author/ai'}/runs/${lastRun}`">打开当前任务</RouterLink>
      <p role="status" aria-live="polite">{{ busy ? '正在提交，请勿重复操作' : '' }}</p>
    </form>
  </div>
</template>
