<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue"
import { useRoute } from "vue-router"
import ArticleSettings from "@/components/editor/ArticleSettings.vue"
import RichMarkdownEditor from "@/components/editor/RichMarkdownEditor.vue"
import MarkdownRenderer from "@/components/article/MarkdownRenderer.vue"
import AppToast from "@/components/shared/AppToast.vue"
import { useApiServices } from "@/request/api-services"
import { createAuthorRuntime, createFakeAuthorRuntime, type AuthorRuntime } from "@/services/author-runtime"
import { createImageLifecycle } from "@/services/image-lifecycle"
import { useEditorDraftStore } from "@/stores/editor-draft"
import { useUiStore } from "@/stores/ui"
import type { ArticleType, Tag } from "@/types/taxonomy"

/** 测试可注入的作者运行时；路由页面默认使用显式 Fake。 */
const props = defineProps<{ readonly runtime?: AuthorRuntime }>()
/** 当前作者运行时；测试保持隔离 Fake，开发可信作者页面使用真实 API。 */
const runtime = props.runtime ?? (import.meta.env.MODE === "test" ? createFakeAuthorRuntime() : createAuthorRuntime(useApiServices()))
/** 当前编辑草稿。 */ const draftStore = useEditorDraftStore()
/** 全局反馈。 */ const ui = useUiStore()
/** 路由参数。 */ const route = useRoute()
/** 图片生命周期。 */ const images = createImageLifecycle(runtime.articles, runtime.guard)
/** 分类选项。 */ const articleTypes = ref<readonly ArticleType[]>([])
/** 标签选项。 */ const tags = ref<readonly Tag[]>([])
/** 初始化状态，失败时保留可重试分支。 */ const initialization = ref<"loading" | "ready" | "error">("loading")

/** 初始化创建或编辑模式。 */
async function initialize(): Promise<void> {
  initialization.value = "loading"
  const id = Number(route.params["id"])
  const articleTypesRequest = Promise.resolve().then(() => runtime.taxonomy.listArticleTypes())
  const tagsRequest = Promise.resolve().then(() => runtime.taxonomy.listTags())
  const articleRequest = Number.isInteger(id) && id > 0 ? Promise.resolve().then(() => runtime.articles.detail(id)) : Promise.resolve(null)
  await Promise.all([articleTypesRequest, tagsRequest, articleRequest]).then(
    ([nextArticleTypes, nextTags, article]) => {
      articleTypes.value = nextArticleTypes; tags.value = nextTags
      if (article === null) draftStore.reset(); else draftStore.load(article)
      initialization.value = "ready"
    },
    () => { initialization.value = "error"; ui.showToast("error", "写作台加载失败", "请重试载入草稿与分类") },
  )
}
onMounted(initialize)
/** 图片取消失败时告知用户由服务端 TTL 继续兜底。 */
const showImageCleanupFallback = (): void => { ui.showToast("error", "图片取消失败", "临时图片将由服务端过期清理") }
/** 离开未保存编辑器时清理临时图片。 */
onBeforeUnmount(() => { void images.cancel().then((cleaned) => { if (!cleaned) showImageCleanupFallback() }) })

/** 保存文章并阻止重复提交。 */
async function save(): Promise<void> {
  if (!draftStore.beginSave()) return
  const id = Number(route.params["id"])
  const write = draftStore.toWrite()
  const result = await Promise.resolve().then(() => runtime.guard.execute("article", () => Number.isInteger(id) && id > 0 ? runtime.articles.update({ id, ...write }) : runtime.articles.create(write))).then(
    (value) => value,
    async () => { draftStore.failSave(); const cleaned = await images.cancel(); if (!cleaned) showImageCleanupFallback(); ui.showToast("error", "保存失败", "草稿仍在，请重试保存"); return null },
  )
  if (result === null) return
  if (!result.ok) { draftStore.failSave(); const cleaned = await images.cancel(); if (!cleaned) showImageCleanupFallback(); ui.showToast("error", "保存被阻止", result.error.reason); return }
  images.commit(); draftStore.finishSave(); ui.showToast("success", "文章已保存")
}
</script>

<template>
  <div class="grid gap-6"><AppToast /><header class="flex items-center justify-between"><div><p class="text-sm text-text-secondary">受保护的作者工作区</p><h1 class="font-display text-3xl font-bold">{{ route.params['id'] ? '编辑文章' : '新建文章' }}</h1></div><button type="button" data-testid="save-article" class="min-w-28 rounded-lg bg-accent px-5 py-3 font-semibold text-white disabled:opacity-60" :disabled="draftStore.saving || initialization !== 'ready'" :aria-busy="draftStore.saving" @click="save">{{ draftStore.saving ? '保存中…' : '保存文章' }}</button></header>
    <p v-if="initialization === 'loading'" role="status" class="rounded-[var(--radius-card)] border border-border bg-surface p-6">正在载入写作台…</p>
    <section v-else-if="initialization === 'error'" role="alert" class="rounded-[var(--radius-card)] border border-error bg-error-soft p-6"><h2 class="font-semibold text-error">写作台加载失败</h2><p class="mt-2 text-text-secondary">草稿与分类尚未载入，请重试。</p><button data-testid="retry-editor-init" class="mt-4 rounded-lg bg-accent px-4 py-3 font-semibold text-white" @click="initialize">重新载入</button></section>
    <template v-else>
      <ArticleSettings :title="draftStore.draft.title" :article-type-id="draftStore.draft.articleTypeId" :tag-ids="draftStore.draft.tagIds" :article-types="articleTypes" :tags="tags" @update:title="draftStore.update({ ...draftStore.draft, title: $event })" @update:article-type-id="draftStore.update({ ...draftStore.draft, articleTypeId: $event })" @update:tag-ids="draftStore.update({ ...draftStore.draft, tagIds: $event })" />
      <RichMarkdownEditor :model-value="draftStore.draft.markdown" :images="images" @update:model-value="draftStore.update({ ...draftStore.draft, markdown: $event })" @upload-failure="ui.showToast('error', '图片上传失败', '正文未发生变化')" />
      <section class="rounded-[var(--radius-card)] border border-border bg-surface p-6"><h2 class="mb-4 text-lg font-semibold">安全预览</h2><MarkdownRenderer :markdown="draftStore.draft.markdown" /></section>
    </template>
  </div>
</template>
