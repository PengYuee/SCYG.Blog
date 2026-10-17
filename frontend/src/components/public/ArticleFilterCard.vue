<script setup lang="ts">
import { CheckIcon, ChevronDownIcon, TagIcon, Squares2X2Icon, XMarkIcon } from "@heroicons/vue/24/outline"
import { computed, nextTick, ref, watch } from "vue"

const props = defineProps<{
  readonly kind: "category" | "tag"
  readonly items: readonly { readonly id: number; readonly name: string }[]
  readonly selectedId?: number | undefined
}>()
const emit = defineEmits<{ change: [id?: number] }>()
const expanded = ref(false)
const options = ref<HTMLElement | null>(null)
const title = computed(() => props.kind === "category" ? "分类" : "标签")
const limit = computed(() => props.kind === "category" ? 6 : 8)
const ordered = computed(() => [
  ...props.items.filter(item => item.id === props.selectedId),
  ...props.items.filter(item => item.id !== props.selectedId),
])
const visible = computed(() => expanded.value ? ordered.value : ordered.value.slice(0, limit.value))

/** 更新前捕获当前焦点，只修复 DOM 移位造成的丢失，不记录等待中的选择请求。 */
watch(() => props.selectedId, async () => {
  const active = document.activeElement
  if (!(active instanceof HTMLElement) || !options.value?.contains(active)) return
  const option = active.closest(".filter-option")?.querySelector<HTMLButtonElement>("[data-filter-id]")
  if (!option) return
  await nextTick()
  if (document.activeElement !== document.body) return
  const target = options.value?.contains(option) ? option : options.value?.querySelector<HTMLButtonElement>("[data-filter-id]")
  target?.focus()
})
</script>

<template>
  <section class="blog-card filter-card" :aria-labelledby="`${kind}-filter-title`">
    <h2 :id="`${kind}-filter-title`" class="filter-title">
      <component :is="kind === 'category' ? Squares2X2Icon : TagIcon" aria-hidden="true" />
      {{ title }}
    </h2>
    <div :id="`${kind}-filter-options`" ref="options" class="filter-options" :class="{ 'filter-options--tags': kind === 'tag', 'filter-options--expanded': expanded }" :aria-label="kind === 'category' ? '文章分类' : '文章标签'">
      <div v-for="item in visible" :key="item.id" class="filter-option" :class="{ 'is-selected': item.id === selectedId }">
        <button type="button" class="filter-name" :data-filter-id="item.id" :title="item.name" :aria-pressed="item.id === selectedId" @click="emit('change', item.id)">
          <CheckIcon v-if="item.id === selectedId" class="filter-check" aria-hidden="true" />
          <span>{{ item.name }}</span>
        </button>
        <button v-if="item.id === selectedId" type="button" class="filter-clear" :aria-label="`取消${title}筛选：${item.name}`" :title="`取消${title}筛选`" @click="emit('change')"><XMarkIcon aria-hidden="true" /></button>
      </div>
      <p v-if="items.length === 0" class="filter-empty">暂无{{ title }}</p>
    </div>
    <button v-if="items.length > limit" type="button" class="filter-toggle" :aria-expanded="expanded" :aria-controls="`${kind}-filter-options`" @click="expanded = !expanded">
      {{ expanded ? "收起" : "展开全部" }}{{ title }}<span v-if="!expanded"> · {{ items.length }}</span>
      <ChevronDownIcon :class="{ 'is-expanded': expanded }" aria-hidden="true" />
    </button>
  </section>
</template>

<style scoped>
.filter-card { padding: var(--space-5); }
.filter-title { display: flex; align-items: center; gap: var(--space-2); margin-bottom: var(--space-3); font-family: var(--font-family-display); font-size: var(--font-size-h3); font-weight: 600; }
.filter-title svg { width: var(--space-5); height: var(--space-5); color: var(--color-text-secondary); }
.filter-options { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--space-2); }
.filter-options--tags { display: flex; flex-wrap: wrap; }
.filter-options--expanded { max-height: calc(var(--space-24) * 3); overflow-y: auto; overscroll-behavior-y: contain; scrollbar-width: thin; padding: var(--space-1); }
.filter-option { display: flex; min-width: 0; max-width: 100%; border-radius: var(--radius-control); color: var(--color-text-secondary); }
.filter-options--tags .filter-option { border-radius: var(--radius-round); background: var(--color-surface-muted); }
.filter-option.is-selected { grid-column: 1 / -1; background: var(--color-accent-soft); color: var(--color-accent); }
.filter-name { display: flex; flex: 1; min-width: 0; align-items: center; gap: var(--space-2); min-height: calc(var(--space-10) + var(--space-1)); padding-inline: var(--space-3); border-radius: inherit; text-align: left; font-size: var(--font-size-small); }
.filter-name span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.is-selected .filter-name span { white-space: normal; overflow-wrap: anywhere; }
.filter-name:hover, .filter-clear:hover { background: var(--color-surface-hover); color: var(--color-text-primary); }
.filter-check { flex-shrink: 0; width: var(--space-4); height: var(--space-4); }
.filter-clear { display: flex; flex-shrink: 0; align-items: center; justify-content: center; width: calc(var(--space-10) + var(--space-1)); min-height: calc(var(--space-10) + var(--space-1)); border-radius: inherit; }
.filter-clear svg { width: var(--space-4); height: var(--space-4); }
.filter-toggle { display: flex; align-items: center; gap: var(--space-1); width: 100%; min-height: var(--space-12); margin-top: var(--space-3); border-top: var(--border-width) solid var(--color-border-subtle); color: var(--color-text-secondary); font-size: var(--font-size-small); }
.filter-toggle:hover { color: var(--color-accent); }
.filter-toggle svg { width: var(--space-4); height: var(--space-4); margin-left: auto; }
.filter-toggle .is-expanded { transform: rotate(180deg); }
.filter-empty { color: var(--color-text-secondary); font-size: var(--font-size-small); }
</style>
