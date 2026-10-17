<script setup lang="ts">
import { computed, ref } from "vue"
import type { AgentRun } from "@/request/generated"
import MarkdownRenderer from "@/components/article/MarkdownRenderer.vue"
import { aiResultModels } from "@/services/ai-business"
const props = defineProps<{ run: AgentRun; anonymous: boolean; original?: string }>()
const search = computed(() => props.run.capability === "search" ? aiResultModels.search.safeParse(props.run.result) : undefined)
const write = computed(() => props.run.capability === "write" ? aiResultModels.write.safeParse(props.run.result) : undefined)
const polish = computed(() => props.run.capability === "polish" ? aiResultModels.polish.safeParse(props.run.result) : undefined)
const chat = computed(() => props.run.capability === "chat" ? aiResultModels.chat.safeParse(props.run.result) : undefined)
const parsed = computed(() => search.value?.success || write.value?.success || polish.value?.success || chat.value?.success)
const copyMessage = ref("")
async function copy(text: string): Promise<void> {
  try { await navigator.clipboard.writeText(text); copyMessage.value = "已复制" } catch { copyMessage.value = "复制失败，请选择文字手动复制" }
}
</script>
<template>
  <section v-if="run.result !== null" class="ai-panel" aria-label="运行结果">
    <h2>运行结果</h2>
    <p v-if="!parsed" role="status">结果结构暂不受此界面支持，未进行推断或转换。请保留 Run 标识用于诊断。</p>
    <template v-if="search?.success">
      <MarkdownRenderer :markdown="search.data.conclusion" />
      <p v-if="search.data.articles.length === 0">本次未返回文章引用。</p>
      <article v-for="article in search.data.articles" :key="article.article_id" class="ai-reference">
        <template v-if="!anonymous || article.visibility === 'published'">
          <RouterLink :to="article.visibility === 'published' ? `/articles/${article.article_id}` : `/author/articles/${article.article_id}/edit`">{{ article.title }}</RouterLink>
          <p v-if="!anonymous">{{ article.visibility }} · #{{ article.article_id }}</p>
          <MarkdownRenderer :markdown="article.evidence" />
        </template>
        <p v-else>服务返回了非公开引用，已阻止展示，请保留 Run 标识用于诊断。</p>
      </article>
    </template>
    <template v-if="write?.success">
      <p class="text-warning">这是尚未保存的生成草稿。复制不会新建、覆盖或发布文章。</p>
      <h3>{{ write.data.title }}</h3><button type="button" @click="copy(write.data.title)">复制标题</button>
      <h3>大纲</h3><MarkdownRenderer :markdown="write.data.outline" /><button type="button" @click="copy(write.data.outline)">复制大纲</button>
      <h3>正文</h3><MarkdownRenderer :markdown="write.data.markdown" /><button type="button" @click="copy(write.data.markdown)">复制正文</button>
      <p v-if="write.data.source_article_ids.length === 0">未使用站内参考。</p>
      <p v-else>来源文章：<RouterLink v-for="id in write.data.source_article_ids" :key="id" :to="`/author/articles/${id}/edit`" class="mr-4">#{{ id }}</RouterLink></p>
    </template>
    <template v-if="polish?.success">
      <p>仅展示润色结果，不会修改原文章。</p>
      <div class="ai-comparison"><section><h3>原文</h3><MarkdownRenderer v-if="original" :markdown="original" /><p v-else>当前 tab 未保存原文；运行快照不包含输入。</p></section><section><h3>润色后</h3><MarkdownRenderer :markdown="polish.data.content" /></section></div>
      <h3>变更说明</h3><MarkdownRenderer :markdown="polish.data.change_summary" /><button type="button" @click="copy(polish.data.content)">复制润色正文</button>
    </template>
    <template v-if="chat?.success">
      <MarkdownRenderer :markdown="chat.data.response" />
      <p v-if="chat.data.source_article_ids.length === 0">未使用站内参考。</p>
      <p v-else>来源：<RouterLink v-for="id in chat.data.source_article_ids" :key="id" :to="`/author/articles/${id}/edit`" class="mr-4">#{{ id }}</RouterLink></p>
    </template>
    <p role="status" aria-live="polite">{{ copyMessage }}</p>
  </section>
  <section v-if="run.failure !== null" class="ai-panel border-error" role="alert"><h2>运行失败</h2><p>{{ run.failure.message }}</p><p>{{ run.failure.code }}</p></section>
</template>
