<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from "vue"
import { ArrowPathIcon } from "@heroicons/vue/24/outline"
import { useApiServices } from "@/request/api-services"
import { ensureAnonymousSession, onAnonymousSessionChange } from "@/services/anonymous-ai-session"
import { createAiRun, isTerminalRun, type AiRunController } from "@/services/ai-run"
import { aiResultModels } from "@/services/ai-business"
import { readTabState } from "@/services/ai-session-state"
import { HttpRequestError } from "@/request/http-error"
import type { ArticleSummary } from "@/types/article"
import ArticleSection from "@/components/article/ArticleSection.vue"
import AppModal from "@/components/shared/AppModal.vue"

const props = defineProps<{ runId: string }>()
const services = useApiServices()
const api = services.ai
const bootError = ref<unknown>(null)
const confirmCancel = ref(false)
const cancelTrigger = ref<HTMLButtonElement | null>(null)
const resultHeading = ref<HTMLElement | null>(null)
const state = ref<AiRunController["state"] | null>(null)
const articles = shallowRef<readonly ArticleSummary[]>([])
const articleLoading = ref(false)
const articleError = ref(false)
const failedArticleIds = shallowRef<readonly number[]>([])
const initialConnectionFinished = ref(false)
const submittedQuery = ref("")
let lifecycle: AiRunController | undefined
let generation = 0
let articleGeneration = 0
let controller: AbortController | undefined
const unsubscribe = onAnonymousSessionChange(() => {
  generation += 1
  articleGeneration += 1
  controller?.abort()
  lifecycle?.stop()
  lifecycle = undefined
  state.value = null
  articles.value = []
  articleLoading.value = false
  initialConnectionFinished.value = false
  bootError.value = new Error("匿名身份已失效，不能使用新凭证访问原任务。")
})
watch(() => props.runId, async (runId) => {
  const own = ++generation
  articleGeneration += 1
  controller?.abort()
  lifecycle?.stop()
  lifecycle = undefined
  controller = new AbortController()
  state.value = null
  articles.value = []
  articleLoading.value = false
  articleError.value = false
  failedArticleIds.value = []
  initialConnectionFinished.value = false
  submittedQuery.value = ""
  bootError.value = null
  if (!/^[A-Za-z0-9._~-]{1,128}$/.test(runId) || runId === "." || runId === "..") { bootError.value = new Error("无效 Run 标识"); return }
  try {
    const identity = await ensureAnonymousSession(api, false, controller.signal)
    if (own !== generation) return
    const storageKey = `scyg.ai.anonymous.${identity.accessToken}.run.${runId}`
    const storedQuery = readTabState<unknown>(`${storageKey}.query`, "")
    submittedQuery.value = typeof storedQuery === "string" ? storedQuery.trim().replace(/\s+/g, " ") : ""
    lifecycle = createAiRun(api, "anonymous", runId, storageKey)
    state.value = lifecycle.state
    await lifecycle.connect()
    if (own === generation) initialConnectionFinished.value = true
  } catch (error) { if (own === generation) bootError.value = error }
}, { immediate: true })
onBeforeUnmount(() => { generation += 1; articleGeneration += 1; controller?.abort(); lifecycle?.stop(); unsubscribe() })
const locked = computed(() => state.value === null || state.value.busy || state.value.now < state.value.waitUntil)
const ended = computed(() => state.value?.run != null && isTerminalRun(state.value.run))
const canRefresh = computed(() => state.value !== null && !ended.value && Boolean(state.value.error || state.value.pendingOperation || (initialConnectionFinished.value && state.value.run === null)))
const search = computed(() => {
  const run = state.value?.run
  return run?.capability === "search" && run.result !== null ? aiResultModels.search.safeParse(run.result) : undefined
})
const working = computed(() => {
  if (bootError.value) return false
  const current = state.value
  if (articleLoading.value || current?.busy) return true
  return !ended.value && !current?.error && !current?.pendingOperation
    && !(current && current.now < current.waitUntil)
    && !(initialConnectionFinished.value && !current?.run)
})
const description = computed(() => {
  if (bootError.value) return "暂时无法读取搜索结果，请刷新页面重试。"
  if (state.value?.busy) return "正在取消搜索…"
  if (state.value?.pendingOperation) return "取消结果尚未确认，请重试。"
  if (articleLoading.value) return "正在整理搜索到的文章…"
  if (state.value?.run?.status === "cancelled") return articles.value.length ? `AI 搜索已取消，已保留 ${articles.value.length} 篇相关文章。` : "AI 搜索已取消。"
  if (state.value?.run?.status === "failed") return articles.value.length ? `AI 搜索未完成，已找到 ${articles.value.length} 篇相关文章。` : "AI 搜索未完成，请在右侧重新搜索。"
  if (!ended.value) {
    if (state.value && state.value.now < state.value.waitUntil) return `搜索繁忙，请在 ${Math.ceil((state.value.waitUntil - state.value.now) / 1000)} 秒后重试。`
    if (initialConnectionFinished.value && !state.value?.run) return "搜索尚未恢复，请重试。"
    return state.value?.error ? "暂时无法更新搜索结果，请重试。" : "正在检索站内相关文章…"
  }
  if (!search.value?.success) return "搜索结果暂时无法展示，请重新搜索。"
  if (articles.value.length === 0) return articleError.value ? "相关文章暂时加载失败，请重试。" : "没有可展示的相关文章，试着换个描述。"
  const topic = submittedQuery.value.length > 40 ? `${submittedQuery.value.slice(0, 40)}…` : submittedQuery.value
  const result = topic ? `AI 找到 ${articles.value.length} 篇与「${topic}」相关的文章` : `AI 找到 ${articles.value.length} 篇相关文章`
  return articleError.value ? `${result}，部分文章暂未加载。` : result
})

/** 公开引用只补全真实文章字段；旧任务、失效身份的迟到响应不能覆盖当前结果。 */
async function loadArticles(retryFailed = false): Promise<void> {
  const result = search.value
  if (!result?.success) return
  const own = ++articleGeneration
  const ids = [...new Set(result.data.articles.filter(article => article.visibility === "published").map(article => article.article_id))]
  const requestedIds = retryFailed ? failedArticleIds.value : ids.filter(id => !articles.value.some(article => article.id === id))
  articleLoading.value = requestedIds.length > 0
  articleError.value = false
  const loaded = await Promise.all(requestedIds.map(async id => {
    try { return { id, article: await services.article.detail(id), error: undefined } }
    catch (error: unknown) { return { id, article: undefined, error } }
  }))
  if (own !== articleGeneration) return
  const available = new Map(articles.value.map(article => [article.id, article]))
  const failed: number[] = []
  for (const item of loaded) {
    if (item.article?.status === 2) available.set(item.id, item.article)
    else if (item.article !== undefined || (item.error instanceof HttpRequestError && item.error.status === 404)) available.delete(item.id)
    else failed.push(item.id)
  }
  articles.value = ids.flatMap(id => { const article = available.get(id); return article === undefined ? [] : [article] })
  failedArticleIds.value = failed
  articleError.value = failed.length > 0
  articleLoading.value = false
}
watch(search, () => {
  articleGeneration += 1
  const result = search.value
  const publicIds = new Set(result?.success ? result.data.articles.filter(article => article.visibility === "published").map(article => article.article_id) : [])
  articles.value = articles.value.filter(article => publicIds.has(article.id))
  articleLoading.value = false
  articleError.value = false
  failedArticleIds.value = []
  void loadArticles()
})
async function refresh(): Promise<void> {
  const current = lifecycle
  const own = generation
  try {
    const run = await current?.refresh()
    if (current !== undefined && run !== undefined && own === generation && lifecycle === current) {
      current.state.error = null
      if (isTerminalRun(run)) current.stop()
      else void current.connect()
    }
  }
  catch (error) { if (current !== undefined && own === generation && lifecycle === current) current.state.error = error }
}
async function cancel(): Promise<void> {
  confirmCancel.value = false
  if (locked.value || ended.value) return
  await lifecycle?.mutate("cancel")
}
async function restoreCancelFocus(): Promise<void> {
  await nextTick()
  if (cancelTrigger.value !== null && !cancelTrigger.value.disabled) cancelTrigger.value.focus()
  else resultHeading.value?.focus()
}
</script>
<template>
  <section aria-label="AI 搜索结果">
    <ArticleSection title="最新文章" :articles="articles" :categories="[]" list>
      <template #description>
        <div class="search-result-summary" :aria-busy="working">
          <p ref="resultHeading" class="search-result-note" tabindex="-1" role="status" aria-live="polite" data-testid="ai-search-status">
            <ArrowPathIcon v-if="working" class="search-result-spinner" aria-hidden="true" />{{ description }}
          </p>
          <button v-if="state && !ended && !state.pendingOperation" ref="cancelTrigger" data-testid="cancel-public-search" type="button" class="search-result-action" :disabled="locked" @click="confirmCancel = true">取消搜索</button>
          <button v-if="state?.pendingOperation" type="button" class="search-result-action" :disabled="locked" @click="lifecycle?.retryPending()">重试取消</button>
          <button v-if="canRefresh" data-testid="retry-public-search" type="button" class="search-result-action" :disabled="locked" @click="refresh">{{ state?.pendingOperation ? "查询结果" : "重试搜索" }}</button>
          <button v-if="articleError" data-testid="retry-search-articles" type="button" class="search-result-action" :disabled="articleLoading" @click="loadArticles(true)">重试文章</button>
        </div>
      </template>
      <template #empty />
    </ArticleSection>
    <AppModal :open="confirmCancel" title="取消这次 AI 搜索？" description="取消后将停止后续处理，已经完成的工作无法撤回。" @close="confirmCancel = false" @closed="restoreCancelFocus">
      <template #footer>
        <button type="button" class="min-h-12 rounded-lg bg-surface-muted px-5 text-text-primary hover:bg-surface-hover" @click="confirmCancel = false">继续搜索</button>
        <button type="button" class="min-h-12 rounded-lg bg-error px-5 font-semibold text-canvas hover:opacity-90 disabled:opacity-50" :disabled="locked || ended" @click="cancel">确认取消任务</button>
      </template>
    </AppModal>
  </section>
</template>
