<script setup lang="ts">
import { onBeforeUnmount, ref } from "vue"
import { useApiServices } from "@/request/api-services"
import type { ArticleDetail } from "@/types/article"
import AiError from "@/components/ai/AiError.vue"
const props = defineProps<{ modelValue: number[]; disabled: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [ids: number[]] }>()
const api = useApiServices().article
const query = ref("")
const items = ref<readonly ArticleDetail[]>([])
const busy = ref(false)
const error = ref<unknown>(null)
const page = ref(0)
const hasNext = ref(false)
let committedQuery = ""
let generation = 0
onBeforeUnmount(() => { generation += 1 })
async function search(next = false): Promise<void> {
  if (busy.value || props.disabled || (next && !hasNext.value)) return
  const own = ++generation
  busy.value = true
  error.value = null
  const requested = next ? page.value + 1 : 0
  if (!next) {
    committedQuery = query.value
    items.value = []
    page.value = 0
    hasNext.value = false
  }
  const requestedQuery = committedQuery
  try {
    const result = await api.manageList({ q: requestedQuery, pageModel: { pageIndex: requested, pageSize: 10 } })
    if (own !== generation) return
    const previous = next ? items.value : []
    const seen = new Set(previous.map((item) => item.id))
    items.value = [...previous, ...result.items.filter((item) => { if (seen.has(item.id)) return false; seen.add(item.id); return true })]
    page.value = requested
    hasNext.value = requested + 1 < result.totalPages
  } catch (failure) { if (own === generation) error.value = failure } finally { if (own === generation) busy.value = false }
}
function toggle(id: number, checked: boolean): void {
  if (!Number.isSafeInteger(id) || id <= 0) return
  emit('update:modelValue', checked ? [...props.modelValue, id] : props.modelValue.filter((value) => value !== id))
}
</script>
<template>
  <fieldset :disabled="disabled" class="ai-reference"><legend>手选站内参考（最多 20 篇）</legend>
    <p>此模式只传递文章 ID，生成器未读取参考原文。需要正文片段请使用“检索参考”模式。</p>
    <label>文章关键词<input v-model="query" type="search" /></label>
    <button type="button" :disabled="busy || disabled" @click="search()">查询真实文章</button>
    <AiError :error="error" />
    <p v-if="!busy && items.length === 0">尚无候选文章；输入关键词后查询。</p>
    <label v-for="article in items" :key="article.id" class="ai-reference flex gap-4"><input type="checkbox" :checked="modelValue.includes(article.id)" :disabled="disabled || (!modelValue.includes(article.id) && modelValue.length >= 20)" @change="toggle(article.id, ($event.target as HTMLInputElement).checked)" /><span>{{ article.title }} · {{ article.status }} · #{{ article.id }}<span class="block">{{ article.digest }}</span></span></label>
    <button v-if="hasNext" type="button" :disabled="busy || disabled" @click="search(true)">加载更多文章</button>
    <p>已选：<button v-for="id in modelValue" :key="id" type="button" :disabled="disabled" @click="toggle(id, false)">移除 #{{ id }}</button><span v-if="modelValue.length === 0">无</span></p>
  </fieldset>
</template>
