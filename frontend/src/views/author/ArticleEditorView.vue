<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue"
import { useRoute } from "vue-router"
import ArticleSettings from "@/components/editor/ArticleSettings.vue"
import RichMarkdownEditor from "@/components/editor/RichMarkdownEditor.vue"
import MarkdownRenderer from "@/components/article/MarkdownRenderer.vue"
import AppToast from "@/components/shared/AppToast.vue"
import { createFakeAuthorRuntime } from "@/services/author-runtime"
import { createImageLifecycle } from "@/services/image-lifecycle"
import { useEditorDraftStore } from "@/stores/editor-draft"
import { useUiStore } from "@/stores/ui"
import type { ArticleType, Tag } from "@/types/taxonomy"

/** 显式 Fake 作者运行时。 */ const runtime = createFakeAuthorRuntime()
/** 当前编辑草稿。 */ const draftStore = useEditorDraftStore()
/** 全局反馈。 */ const ui = useUiStore()
/** 路由参数。 */ const route = useRoute()
/** 图片生命周期。 */ const images = createImageLifecycle(runtime.articles, runtime.guard)
/** 分类选项。 */ const articleTypes = ref<readonly ArticleType[]>([])
/** 标签选项。 */ const tags = ref<readonly Tag[]>([])

/** 初始化创建或编辑模式。 */
onMounted(async () => { articleTypes.value = await runtime.taxonomy.listArticleTypes(); tags.value = await runtime.taxonomy.listTags(); const id = Number(route.params["id"]); if (Number.isInteger(id) && id > 0) draftStore.load(await runtime.articles.detail(id)); else draftStore.reset() })
/** 离开未保存编辑器时清理临时图片。 */
onBeforeUnmount(() => { void images.cancel() })

/** 保存文章并阻止重复提交。 */
async function save(): Promise<void> {
  if (!draftStore.beginSave()) return
  const id = Number(route.params["id"])
  const write = draftStore.toWrite()
  const result = await runtime.guard.execute("article", () => Number.isInteger(id) && id > 0 ? runtime.articles.update({ id, ...write }) : runtime.articles.create(write))
  if (!result.ok) { draftStore.failSave(); ui.showToast("error", "保存被阻止", result.error.reason); return }
  images.commit(); draftStore.finishSave(); ui.showToast("success", "文章已保存")
}
</script>

<template>
  <div class="grid gap-6"><AppToast /><header class="flex items-center justify-between"><div><p class="text-sm text-text-secondary">受保护的作者工作区</p><h1 class="font-display text-3xl font-bold">{{ route.params['id'] ? '编辑文章' : '新建文章' }}</h1></div><button type="button" data-testid="save-article" class="min-w-28 rounded-lg bg-accent px-5 py-3 font-semibold text-white disabled:opacity-60" :disabled="draftStore.saving" :aria-busy="draftStore.saving" @click="save">{{ draftStore.saving ? '保存中…' : '保存文章' }}</button></header>
    <ArticleSettings :title="draftStore.draft.title" :article-type-id="draftStore.draft.articleTypeId" :tag-ids="draftStore.draft.tagIds" :article-types="articleTypes" :tags="tags" @update:title="draftStore.update({ ...draftStore.draft, title: $event })" @update:article-type-id="draftStore.update({ ...draftStore.draft, articleTypeId: $event })" @update:tag-ids="draftStore.update({ ...draftStore.draft, tagIds: $event })" />
    <RichMarkdownEditor :model-value="draftStore.draft.markdown" :images="images" @update:model-value="draftStore.update({ ...draftStore.draft, markdown: $event })" @upload-failure="ui.showToast('error', '图片上传失败', '正文未发生变化')" />
    <section class="rounded-[var(--radius-card)] border border-border bg-surface p-6"><h2 class="mb-4 text-lg font-semibold">安全预览</h2><MarkdownRenderer :markdown="draftStore.draft.markdown" /></section>
  </div>
</template>
