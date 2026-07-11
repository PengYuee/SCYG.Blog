<script setup lang="ts">
import { computed, useId } from "vue"
import { MdCatalog, MdPreview } from "md-editor-v3"
import "md-editor-v3/lib/preview.css"
import { sanitizeMarkdown } from "@/security/sanitize-markdown"

/** Markdown 阅读器输入属性。 */
type Props = {
  /** 来自文章接口或未来编辑器的未受信任 Markdown。 */
  readonly markdown: string
}

const { markdown } = defineProps<Props>()

/** 当前组件实例独有的预览与目录 DOM 标识。 */
const previewId = `article-markdown-${useId()}`

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