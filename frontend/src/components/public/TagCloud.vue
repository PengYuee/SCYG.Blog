<script setup lang="ts">
import { TagIcon } from "@heroicons/vue/24/outline"
import { RouterLink } from "vue-router"
import type { Tag } from "@/types/taxonomy"

/** 标签云输入属性。 */
type Props = {
  /** 可用标签。 */ readonly tags: readonly Tag[]
  /** 当前选中的标签。 */ readonly activeTagId?: number
}

defineProps<Props>()
</script>

<template>
  <section class="blog-card p-6" aria-labelledby="tag-cloud-title">
    <div class="mb-4 flex items-center gap-3">
      <TagIcon class="size-5 text-accent" aria-hidden="true" />
      <h2 id="tag-cloud-title" class="font-[family-name:var(--font-family-display)] text-xl font-semibold text-text-primary">标签</h2>
    </div>
    <div class="flex flex-wrap gap-2">
      <RouterLink v-for="tag in tags" :key="tag.id" :to="{ path: '/articles', query: { tagId: String(tag.id) } }" class="inline-flex min-h-11 items-center rounded-full border px-4 text-sm font-medium transition" :class="tag.id === activeTagId ? 'border-accent bg-accent-soft text-accent' : 'border-border bg-surface text-text-secondary hover:border-accent hover:text-accent'">{{ tag.name }}</RouterLink>
      <p v-if="tags.length === 0" class="text-sm text-text-secondary">暂无标签</p>
    </div>
  </section>
</template>