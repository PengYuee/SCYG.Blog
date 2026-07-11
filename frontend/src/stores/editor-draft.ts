import { defineStore } from "pinia"
import { computed, ref } from "vue"
import type { ArticleDetail, ArticleWrite } from "@/types/article"

/** 编辑器草稿输入。 */
export type EditorDraft = {
  /** 标题。 */ readonly title: string
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
  /** 当前草稿。 */ const draft = ref<EditorDraft>({ title: "", markdown: "", articleTypeId: 0, tagIds: [] })
  /** 最近保存快照。 */ const saved = ref<EditorDraft>({ ...draft.value })
  /** 是否正在保存。 */ const saving = ref(false)
  /** 草稿是否变化。 */ const dirty = computed(() => JSON.stringify(draft.value) !== JSON.stringify(saved.value))
  /** 装载创建模式空草稿。 */ const reset = (): void => { draft.value = { title: "", markdown: "", articleTypeId: 0, tagIds: [] }; saved.value = { ...draft.value } }
  /** 将 T3 领域文章精确映射到编辑草稿。 */ const load = (article: ArticleDetail): void => { draft.value = { title: article.title, markdown: article.markdown, articleTypeId: article.articleTypeId, tagIds: article.tagIds }; saved.value = { ...draft.value } }
  /** 更新当前草稿。 */ const update = (next: EditorDraft): void => { draft.value = next }
  /** 生成 T3 写入模型，并在持久化边界清除 Base64 图片。 */ const toWrite = (): ArticleWrite => { const markdown = purgeDataImages(draft.value.markdown); return { ...draft.value, markdown, digest: digestMarkdown(markdown) } }
  /** 标记保存开始并阻止重复提交。 */ const beginSave = (): boolean => { if (saving.value) return false; saving.value = true; return true }
  /** 标记保存成功。 */ const finishSave = (): void => { saved.value = { ...draft.value }; saving.value = false }
  /** 标记保存失败并保留草稿。 */ const failSave = (): void => { saving.value = false }
  return { draft, saving, dirty, reset, load, update, toWrite, beginSave, finishSave, failSave }
})
