<script setup lang="ts">
import type { ArticleType, Tag } from "@/types/taxonomy"

/** 文章设置属性。 */
defineProps<{ readonly title: string; readonly articleTypeId: number; readonly tagIds: readonly number[]; readonly articleTypes: readonly ArticleType[]; readonly tags: readonly Tag[] }>()
/** 文章设置更新事件。 */
defineEmits<{ "update:title": [value: string]; "update:articleTypeId": [value: number]; "update:tagIds": [value: readonly number[]] }>()
</script>

<template>
  <section class="grid gap-4 rounded-[var(--radius-card)] border border-border bg-surface p-6" aria-labelledby="article-settings-title">
    <h2 id="article-settings-title" class="text-lg font-semibold">文章设置</h2>
    <label class="grid gap-2 text-sm font-medium">标题<input :value="title" class="h-11 rounded-lg border border-border px-3" data-testid="article-title" @input="$emit('update:title', ($event.target as HTMLInputElement).value)" /></label>
    <label class="grid gap-2 text-sm font-medium">分类<select :value="articleTypeId" class="h-11 rounded-lg border border-border px-3" @change="$emit('update:articleTypeId', Number(($event.target as HTMLSelectElement).value))"><option :value="0">请选择分类</option><option v-for="item in articleTypes" :key="item.id" :value="item.id">{{ item.name }}</option></select></label>
    <fieldset><legend class="mb-2 text-sm font-medium">标签</legend><div class="flex flex-wrap gap-3"><label v-for="tag in tags" :key="tag.id" class="flex min-h-11 items-center gap-2 rounded-lg border border-border px-3"><input type="checkbox" :checked="tagIds.includes(tag.id)" @change="$emit('update:tagIds', ($event.target as HTMLInputElement).checked ? [...tagIds, tag.id] : tagIds.filter((id) => id !== tag.id))" />{{ tag.name }}</label></div></fieldset>
  </section>
</template>
