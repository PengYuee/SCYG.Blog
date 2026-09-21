import { defineStore } from "pinia"
import { computed, ref } from "vue"
import type { ArticleCreateRequest, ArticleDetail, ArticlePatchRequest } from "@/types/article"
import { pinyin } from "pinyin-pro"
/** 根据标题生成可用于文章地址的简短名称；中文标题片段转换为拼音且允许作者覆盖。 */
export function generateArticleSlug(title: string): string {
  const transliterated = title.replace(/[\u3400-\u9fff]+/g, (segment) => pinyin(segment, { toneType: "none" }))
  const normalized = transliterated.normalize("NFKD").toLowerCase().replace(/[\u0300-\u036f]/g, "")
  const generated = normalized.replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 160).replace(/-+$/g, "")
  return generated || (title.trim() === "" ? "" : "new-article")
}

/** 编辑器草稿输入。 */
export type EditorDraft = {
  /** 标题。 */ readonly title: string
  /** 文章链接名；服务端字段仍使用 slug。 */ readonly slug: string
  /** Markdown 正文。 */ readonly markdown: string
  /** 分类标识。 */ readonly articleTypeId: number
  /** 标签标识。 */ readonly tagIds: readonly number[]
}

/** 从 Markdown 行结构生成纯文本摘要，不解析 HTML。 */
export function digestMarkdown(markdown: string, limit = 160): string {
  return markdown.split("\n")
    .filter((line) => !line.trimStart().startsWith("```"))
    .map((line) => line.replace(/^\s{0,3}(?:#{1,6}|>|[-+*]|\d+[.)])\s+/, "").replace(/!\[[^\]]*\]\([^)]*\)/g, "").replace(/\[([^\]]+)\]\([^)]*\)/g, "$1").replace(/[*_`~]/g, "").trim())
    .filter(Boolean).join(" ").slice(0, limit)
}

/** 移除 Markdown 图片语法中的 data URL，保证草稿不会持久化 Base64。 */
export function purgeDataImages(markdown: string): string {
  return markdown.replace(/!\[[^\]]*\]\(\s*data:[^)]+\)/gi, "")
}

/** 编辑器草稿状态仓库。 */
export const useEditorDraftStore = defineStore("editor-draft", () => {
  const emptyDraft = (): EditorDraft => ({ title: "", slug: "", markdown: "", articleTypeId: 0, tagIds: [] })
  /** 当前草稿。 */ const draft = ref<EditorDraft>(emptyDraft())
  /** 最近保存快照。 */ const saved = ref<EditorDraft>({ ...draft.value })
  /** 编辑器是否正在保存。 */ const saving = ref(false)
  /** 当前服务端文章版本；创建模式尚无版本。 */ const version = ref<number | undefined>(undefined)
  /** 新建文章的链接名是否已被作者手动覆盖。 */ const slugCustomized = ref(false)
  /** 草稿是否变化。 */ const dirty = computed(() => JSON.stringify(draft.value) !== JSON.stringify(saved.value))
  /** 装载创建模式空草稿。 */
  const reset = (): void => { draft.value = emptyDraft(); saved.value = { ...draft.value }; version.value = undefined; slugCustomized.value = false }
  /** 将服务端文章精确映射到编辑草稿和并发版本。 */
  const load = (article: ArticleDetail): void => {
    draft.value = { title: article.title, slug: article.slug, markdown: article.markdown, articleTypeId: article.articleTypeId, tagIds: [...article.tagIds] }
    saved.value = { ...draft.value, tagIds: [...draft.value.tagIds] }
    version.value = article.version
    slugCustomized.value = true
  }
  /** 更新标题；新建文章尚未手动修改链接名时同步自动生成。 */
  const updateTitle = (title: string): void => { draft.value = { ...draft.value, title, slug: slugCustomized.value ? draft.value.slug : generateArticleSlug(title) } }
  /** 更新文章链接名并切换为手动覆盖模式。 */
  const updateSlug = (slug: string): void => { slugCustomized.value = true; draft.value = { ...draft.value, slug } }
  /** 更新当前草稿。 */
  const update = (next: EditorDraft): void => { draft.value = next }
  /** 生成服务端创建模型，并在持久化边界清除 Base64 图片。 */
  const toCreate = (): ArticleCreateRequest => {
    const markdown = purgeDataImages(draft.value.markdown)
    return { title: draft.value.title, slug: draft.value.slug, markdown, digest: digestMarkdown(markdown), articleTypeId: draft.value.articleTypeId, tagIds: [...draft.value.tagIds], status: 1 }
  }
  /** 生成服务端局部更新字段，并在持久化边界清除 Base64 图片。 */
  const toPatch = (): ArticlePatchRequest => {
    const markdown = purgeDataImages(draft.value.markdown)
    return { title: draft.value.title, slug: draft.value.slug, markdown, digest: digestMarkdown(markdown), articleTypeId: draft.value.articleTypeId, tagIds: [...draft.value.tagIds] }
  }
  /** 用服务端返回资源收敛草稿、保存快照和并发版本。 */
  const finishSave = (article: ArticleDetail): void => {
    draft.value = { title: article.title, slug: article.slug, markdown: article.markdown, articleTypeId: article.articleTypeId, tagIds: [...article.tagIds] }
    saved.value = { ...draft.value, tagIds: [...draft.value.tagIds] }
    version.value = article.version
    saving.value = false
  }
  /** 标记保存开始并阻止重复提交。 */
  const beginSave = (): boolean => { if (saving.value) return false; saving.value = true; return true }
  /** 标记保存失败并保留草稿。 */
  const failSave = (): void => { saving.value = false }
  return { draft, saved, version, saving, dirty, reset, load, updateTitle, updateSlug, update, toCreate, toPatch, beginSave, finishSave, failSave }
})
