<script setup lang="ts">
import { computed } from "vue"
import { MdCatalog, MdPreview } from "md-editor-v3"
import "md-editor-v3/lib/preview.css"
import { sanitizeMarkdown } from "@/security/sanitize-markdown"

/** Markdown 阅读器输入属性。 */
type Props = {
  /** 来自文章接口或未来编辑器的未受信任 Markdown。 */
  readonly markdown: string
  /** 预览与目录共享的稳定 DOM 标识。 */
  readonly previewId?: string
}

/** 响应式解构确保可选标识在模板中始终为字符串。 */
const { markdown, previewId = "article-markdown-preview" } = defineProps<Props>()

/** 目录与预览共同消费的安全 Markdown 源，阻止原始 HTML 标题绕过安全边界。 */
const sanitizedMarkdown = computed(
  /**
   * 清理当前 Markdown 源。
   * @returns 可交给 md-editor-v3 解析和提取目录的安全源。
   */
  () => sanitizeMarkdown(markdown),
)
</script>

<template>
  <div class="grid gap-8 lg:grid-cols-[minmax(0,1fr)_16rem]">
    <MdPreview
      :id="previewId"
      :model-value="sanitizedMarkdown"
      :sanitize="sanitizeMarkdown"
      language="zh-CN"
      preview-theme="github"
    />
    <aside class="border-l border-border-subtle pl-6" aria-label="文章目录">
      <MdCatalog :editor-id="previewId" scroll-element="html" />
    </aside>
  </div>
</template>
