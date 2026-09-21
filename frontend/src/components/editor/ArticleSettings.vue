<script setup lang="ts">
import { PencilSquareIcon, PlusIcon, XCircleIcon } from "@heroicons/vue/24/outline"
import ArticleTypeDropdown from "@/components/editor/ArticleTypeDropdown.vue"
import type { AuthorArticleType, AuthorTag } from "@/services/author-contracts"

/** 文章设置属性。 */
type Props = {
  /** 父级控制的文章标题。 */ readonly title: string
  /** 父级控制的文章链接名。 */ readonly slug: string
  /** 父级控制的文章分类标识。 */ readonly articleTypeId: number
  /** 父级控制的标签标识。 */ readonly tagIds: readonly number[]
  /** 作者可管理的版本化分类。 */ readonly articleTypes: readonly AuthorArticleType[]
  /** 作者可选择和删除的版本化标签。 */ readonly tags: readonly AuthorTag[]
  /** 父级控制的分类与标签变更锁。 */ readonly disabled?: boolean
}

/** 文章设置受控属性与分类、标签变更锁默认值。 */
const props = withDefaults(defineProps<Props>(), { disabled: false })
/** 文章设置更新与分类、标签管理请求事件。 */
const emit = defineEmits<{
  "update:title": [value: string]
  "update:slug": [value: string]
  "update:articleTypeId": [value: number]
  "update:tagIds": [value: readonly number[]]
  "request-create-article-type": []
  "request-update-article-type": [articleType: AuthorArticleType]
  "request-delete-article-type": [articleType: AuthorArticleType]
  "request-create-tag": []
  "request-update-tag": [tag: AuthorTag]
  "request-delete-tag": [tag: AuthorTag]
}>()

/** 发出新增标签请求，变更锁启用时保持静默。 */
const requestCreateTag = (): void => {
  if (props.disabled) return
  emit("request-create-tag")
}

/** 发出完整版本化标签删除请求，不改变受控选择。 */
const requestDeleteTag = (tag: AuthorTag): void => {
  if (props.disabled) return
  emit("request-delete-tag", tag)
}

/** 根据原生复选框状态生成新的受控标签标识集合。 */
const updateTagSelection = (tagId: number, event: Event): void => {
  if (props.disabled || !(event.target instanceof HTMLInputElement)) return
  emit("update:tagIds", event.target.checked ? [...props.tagIds, tagId] : props.tagIds.filter((id) => id !== tagId))
}
</script>
<template>
  <section class="grid gap-4 rounded-[var(--radius-card)] border border-border bg-surface p-6" aria-labelledby="article-settings-title">
    <h2 id="article-settings-title" class="text-lg font-semibold">文章设置</h2>
    <label class="grid gap-2 text-sm font-medium">标题<input :value="title" class="h-11 rounded-lg border border-border px-3" data-testid="article-title" @input="$emit('update:title', ($event.target as HTMLInputElement).value)" /></label>
    <label class="grid gap-2 text-sm font-medium">文章链接名<input :value="slug" class="h-11 rounded-lg border border-border px-3" data-testid="article-slug" pattern="[a-z0-9]+(?:-[a-z0-9]+)*" maxlength="160" aria-describedby="article-link-name-help" @input="$emit('update:slug', ($event.target as HTMLInputElement).value)" /><span id="article-link-name-help" class="text-xs font-normal text-text-secondary">根据标题自动生成，也可以手动修改。仅支持小写字母、数字和连字符。</span></label>
    <ArticleTypeDropdown
      :article-type-id="articleTypeId"
      :article-types="articleTypes"
      :disabled="props.disabled"
      @update:article-type-id="$emit('update:articleTypeId', $event)"
      @request-create-article-type="$emit('request-create-article-type')"
      @request-update-article-type="$emit('request-update-article-type', $event)"
      @request-delete-article-type="$emit('request-delete-article-type', $event)"
    />
    <fieldset class="grid gap-2">
      <legend class="sr-only">标签</legend>
      <div class="flex min-h-11 flex-wrap items-center justify-between gap-3">
        <span class="text-sm font-medium" aria-hidden="true">标签</span>
        <button
          type="button"
          class="flex min-h-11 items-center gap-2 rounded-[var(--radius-control)] border border-border bg-surface px-3 text-accent transition-colors duration-[var(--duration-fast)] hover:bg-accent-soft disabled:cursor-not-allowed disabled:opacity-60"
          :disabled="disabled"
          aria-label="新增标签"
          @click="requestCreateTag"
        >
          <PlusIcon class="size-5" aria-hidden="true" />
          新增标签
        </button>
      </div>
      <div class="flex flex-wrap gap-3">
        <div v-for="tag in tags" :key="tag.id" class="flex min-h-11 overflow-hidden rounded-[var(--radius-control)] border border-border">
          <label class="flex min-h-11 items-center gap-2 px-3">
            <input
              type="checkbox"
              :checked="tagIds.includes(tag.id)"
              :disabled="disabled"
              @change="updateTagSelection(tag.id, $event)"
            />
            <span>{{ tag.name }}</span>
          </label>
          <button type="button" class="flex min-h-11 min-w-11 items-center justify-center border-l border-border text-text-secondary transition-colors duration-[var(--duration-fast)] hover:bg-accent-soft hover:text-accent disabled:cursor-not-allowed disabled:opacity-60" :disabled="disabled" :aria-label="`修改标签“${tag.name}”`" @click="emit('request-update-tag', tag)">
            <PencilSquareIcon class="size-5" aria-hidden="true" />
          </button>
          <button type="button" class="flex min-h-11 min-w-11 items-center justify-center border-l border-border text-text-secondary transition-colors duration-[var(--duration-fast)] hover:bg-error-soft hover:text-error disabled:cursor-not-allowed disabled:opacity-60" :disabled="disabled" :aria-label="`删除标签“${tag.name}”`" @click="requestDeleteTag(tag)">
            <XCircleIcon class="size-5" aria-hidden="true" />
          </button>
        </div>
      </div>
    </fieldset>
  </section>
</template>
