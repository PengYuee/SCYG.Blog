<script setup lang="ts">
import { MagnifyingGlassIcon, SparklesIcon, ArrowUpIcon, AdjustmentsHorizontalIcon } from "@heroicons/vue/24/outline"
import { computed, nextTick, ref } from "vue"

const props = withDefaults(defineProps<{
  readonly compact?: boolean
  readonly surface?: boolean
  readonly sidebar?: boolean
  readonly inputId?: string
  readonly busy?: boolean
  readonly locked?: boolean
  readonly canSubmit?: boolean
  readonly queryMaxChars?: number
  readonly resultLimit?: number
  readonly retrying?: boolean
}>(), { compact: false, inputId: "hero-search-input", busy: false, locked: false, canSubmit: true, queryMaxChars: 2000, resultLimit: 5, retrying: false })
const mode = defineModel<"normal" | "ai">("mode", { default: "normal" })
const keyword = defineModel<string>("keyword", { default: "" })
const question = defineModel<string>("question", { default: "" })
const maxResults = defineModel<number>("maxResults", { default: 5 })
const emit = defineEmits<{ submit: [] }>()
const expanded = ref(false)
const optionsOpen = ref(false)
const optionsTrigger = ref<HTMLButtonElement | null>(null)
const showQuestion = computed(() => mode.value === "ai" && (!props.compact || expanded.value))
const label = computed(() => props.busy ? "提交中…" : props.retrying ? "重试原操作" : mode.value === "ai" ? "AI 搜索" : "搜索文章")
async function expandQuestion(): Promise<void> {
  expanded.value = true
  await nextTick()
  document.getElementById(props.inputId)?.focus()
}
async function collapseQuestion(): Promise<void> {
  expanded.value = false
  await nextTick()
  document.getElementById(props.inputId)?.focus()
}

function submit(): void {
  if (mode.value === "ai" && (!props.canSubmit || props.busy)) return
  emit("submit")
  if (props.compact && expanded.value) void collapseQuestion()
}
function questionKeydown(event: KeyboardEvent): void {
  if (event.isComposing) return
  if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) { event.preventDefault(); submit() }
}
async function escapeSearch(event: KeyboardEvent): Promise<void> {
  if (event.isComposing || mode.value !== "ai") return
  if (optionsOpen.value) {
    event.preventDefault()
    event.stopPropagation()
    optionsOpen.value = false
    await nextTick()
    optionsTrigger.value?.focus()
  } else if (props.compact && expanded.value) {
    event.preventDefault()
    void collapseQuestion()
  }
}
</script>

<template>
  <form class="unified-search" :class="{ 'unified-search--compact': compact, 'unified-search--surface': surface, 'unified-search--sidebar': sidebar, 'unified-search--expanded': compact && showQuestion }" role="search" aria-label="站内搜索" @submit.prevent="submit" @keydown.esc="escapeSearch">
    <div class="search-modes" role="group" aria-label="搜索方式">
      <button type="button" :aria-pressed="mode === 'normal'" @click="mode = 'normal'; expanded = false"><MagnifyingGlassIcon aria-hidden="true" />普通搜索</button>
      <button type="button" :aria-pressed="mode === 'ai'" @click="mode = 'ai'"><SparklesIcon aria-hidden="true" />AI 搜索</button>
    </div>
    <div class="search-body">
      <label class="search-field-label" :for="inputId">{{ mode === "ai" ? "搜索问题" : "文章关键词" }}</label>
      <template v-if="mode === 'normal'">
        <MagnifyingGlassIcon class="search-leading" aria-hidden="true" />
        <input :id="inputId" v-model="keyword" type="search" placeholder="搜索文章标题或摘要…" autocomplete="off" />
      </template>
      <textarea v-else-if="showQuestion" :id="inputId" v-model="question" :maxlength="queryMaxChars" :disabled="locked" rows="3" placeholder="描述你想查找的文章主题…" @keydown="questionKeydown" />
      <button v-else :id="inputId" class="compact-question" type="button" @click="expandQuestion">{{ question || "描述你想了解的问题…" }}</button>
      <div v-if="mode === 'ai' && showQuestion" class="search-ai-footer">
        <button ref="optionsTrigger" type="button" class="search-options-trigger" :aria-expanded="optionsOpen" @click="optionsOpen = !optionsOpen"><AdjustmentsHorizontalIcon aria-hidden="true" />搜索选项</button>
        <span class="search-shortcut">Ctrl / ⌘ + Enter</span>
        <button type="submit" class="search-submit" :aria-label="label" :title="label" :disabled="!canSubmit || busy"><ArrowUpIcon aria-hidden="true" /><span>{{ label }}</span></button>
      </div>
      <button v-else type="submit" class="search-submit" :aria-label="label" :title="label" :disabled="mode === 'ai' && (!canSubmit || busy)"><component :is="mode === 'ai' ? SparklesIcon : MagnifyingGlassIcon" aria-hidden="true" /><span>{{ label }}</span></button>
    </div>
    <div v-if="mode === 'ai' && optionsOpen && showQuestion" class="search-options">
      <label :for="`${inputId}-limit`">最多参考文章</label>
      <input :id="`${inputId}-limit`" v-model.number="maxResults" type="number" min="1" :max="resultLimit" :disabled="locked" />
      <span>问题最多 {{ queryMaxChars }} 字符</span>
    </div>
    <p v-if="!compact && !sidebar" class="search-hint">{{ mode === "ai" ? "根据你的描述查找站内文章 · 无需登录" : "从一个关键词开始，找到值得阅读的内容" }}</p>
    <button v-if="compact && showQuestion" type="button" class="search-collapse" @click="collapseQuestion">收起</button>
  </form>
</template>

<style scoped>
.unified-search { position: relative; width: 100%; max-width: var(--layout-reading-measure); color: var(--color-hero-text); }
.unified-search:not(.unified-search--compact) { min-height: calc(var(--space-24) * 3 + var(--space-2)); }
.unified-search button { transition: background var(--duration-fast), color var(--duration-fast), transform var(--duration-fast), opacity var(--duration-fast); }
.unified-search button:active:not(:disabled) { transform: scale(var(--interaction-scale-pressed)); }
.search-modes { display: flex; justify-content: center; gap: var(--space-1); margin-bottom: var(--space-4); }
.search-modes button { display: inline-flex; align-items: center; justify-content: center; gap: var(--space-2); min-height: var(--space-12); padding-inline: var(--space-5); border-radius: var(--radius-round); color: var(--color-hero-text-muted); }
.search-modes button[aria-pressed="true"] { background: var(--color-hero-surface-active); color: var(--color-hero-text); box-shadow: var(--shadow-card); }
.search-modes button:hover { background: var(--color-hero-surface-hover); color: var(--color-hero-text); }
.unified-search svg { width: var(--space-5); height: var(--space-5); flex-shrink: 0; }
.search-body { position: relative; display: flex; flex-wrap: wrap; align-items: center; padding: var(--space-2); border: var(--border-width) solid var(--color-hero-border); border-radius: var(--radius-panel); background: var(--color-hero-surface); backdrop-filter: blur(var(--space-3)); box-shadow: var(--shadow-card); }
.search-field-label { position: absolute; top: calc(var(--space-2) * -1); left: var(--space-5); padding-inline: var(--space-2); border-radius: var(--radius-control); background: var(--color-hero-surface-active); color: var(--color-hero-text); font-size: var(--font-size-caption); line-height: var(--line-height-caption); }
.search-leading { margin-left: var(--space-3); color: var(--color-hero-text-muted); }
.search-body input { flex: 1; min-width: 0; min-height: var(--space-12); padding-inline: var(--space-3); background: transparent; outline: none; }
.search-body textarea { width: 100%; resize: vertical; min-height: var(--space-24); max-height: calc(var(--space-24) * 2); padding: var(--space-3); background: transparent; outline: none; line-height: var(--line-height-body); }
.search-body input::placeholder, .search-body textarea::placeholder { color: var(--color-hero-text-muted); }
.search-body:focus-within { outline: var(--focus-outline-width) solid var(--color-accent); outline-offset: var(--focus-outline-offset); }
.search-submit { display: inline-flex; align-items: center; justify-content: center; gap: var(--space-2); min-height: var(--space-12); padding-inline: var(--space-5); border-radius: var(--radius-card); background: var(--color-accent); color: var(--color-canvas); font-size: var(--font-size-small); font-weight: 600; white-space: nowrap; }
.search-submit:hover { background: var(--color-accent-hover); }
.search-submit:disabled { opacity: 0.55; cursor: not-allowed; }
.search-ai-footer { display: flex; align-items: center; width: 100%; gap: var(--space-3); }
.search-options-trigger { display: inline-flex; align-items: center; gap: var(--space-2); min-height: var(--space-12); padding-inline: var(--space-3); color: var(--color-hero-text-muted); font-size: var(--font-size-small); }
.search-options-trigger:hover { border-radius: var(--radius-control); background: var(--color-hero-surface-hover); color: var(--color-hero-text); }
.search-shortcut { margin-left: auto; color: var(--color-hero-text-muted); font-size: var(--font-size-caption); }
.search-options { position: absolute; left: var(--space-2); right: var(--space-2); bottom: calc(var(--space-24) + var(--space-12)); display: flex; align-items: center; gap: var(--space-3); padding: var(--space-3); font-size: var(--font-size-small); border: var(--border-width) solid var(--color-hero-border); border-radius: var(--radius-control); background: var(--color-hero-surface-active); box-shadow: var(--shadow-dialog); backdrop-filter: blur(var(--space-3)); }
.search-options input { width: var(--space-20); min-height: var(--space-12); padding-inline: var(--space-3); border-radius: var(--radius-control); background: var(--color-surface); color: var(--color-text-primary); }
.search-options span { margin-left: auto; color: var(--color-hero-text-muted); }
.search-hint { display: table; margin: var(--space-3) auto 0; padding: var(--space-1) var(--space-3); border-radius: var(--radius-round); background: var(--color-hero-surface); backdrop-filter: blur(var(--space-3)); text-align: center; font-size: var(--font-size-small); color: var(--color-hero-text); }
.unified-search--compact { display: flex; align-items: center; gap: var(--space-3); color: var(--color-text-primary); position: relative; }
.unified-search--compact .search-modes { flex-shrink: 0; margin: 0; }
.unified-search--compact .search-modes button { padding-inline: var(--space-3); min-height: var(--space-12); font-size: var(--font-size-small); color: var(--color-text-secondary); }
.unified-search--compact .search-modes button[aria-pressed="true"] { background: var(--color-accent-soft); color: var(--color-accent); box-shadow: none; }
.unified-search--compact .search-modes button:hover { background: var(--color-surface-hover); }
.unified-search--compact .search-body { flex: 1; min-width: 0; padding: var(--space-1); border-radius: var(--radius-control); background: var(--color-surface-muted); border-color: var(--color-border-subtle); box-shadow: none; backdrop-filter: none; }
.unified-search--compact .search-field-label { left: var(--space-3); background: var(--color-surface); color: var(--color-text-secondary); }
.unified-search--compact .search-leading { display: none; }
.unified-search--compact .search-body input, .unified-search--compact .search-submit { min-height: var(--space-10); }
.unified-search--compact .search-submit { padding-inline: var(--space-3); border-radius: var(--radius-control); }
.unified-search--compact .search-submit span { display: none; }
.unified-search--compact .search-body input::placeholder, .unified-search--compact .search-body textarea::placeholder { color: var(--color-text-tertiary); }
.compact-question { flex: 1; min-width: 0; min-height: var(--space-10); padding-inline: var(--space-3); text-align: left; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: var(--color-text-secondary); }
.compact-question:hover, .search-collapse:hover, .unified-search--compact .search-options-trigger:hover { border-radius: var(--radius-control); background: var(--color-surface-hover); color: var(--color-text-primary); }
.unified-search--expanded .search-body { position: absolute; top: calc(var(--space-12) + var(--space-2)); left: 0; right: 0; background: var(--color-surface); border-color: var(--color-border); box-shadow: var(--shadow-dialog); }
.unified-search--expanded .search-shortcut, .unified-search--expanded .search-options-trigger { color: var(--color-text-secondary); }
.unified-search--expanded .search-options { top: calc(var(--space-24) * 2 + var(--space-4)); bottom: auto; left: 0; right: 0; background: var(--color-surface); border-color: var(--color-border); color: var(--color-text-primary); box-shadow: var(--shadow-dialog); }
.unified-search--expanded .search-options span { color: var(--color-text-secondary); }
.search-collapse { margin-left: auto; min-height: var(--space-12); color: var(--color-text-secondary); font-size: var(--font-size-small); }
.unified-search.unified-search--surface { min-height: 0; color: var(--color-text-primary); }
.unified-search--surface .search-body, .unified-search--surface .search-hint { background: var(--color-surface); border-color: var(--color-border); color: var(--color-text-primary); backdrop-filter: none; }
.unified-search--surface .search-field-label { background: var(--color-surface); color: var(--color-text-secondary); }
.unified-search--surface .search-modes button, .unified-search--surface .search-leading, .unified-search--surface .search-shortcut, .unified-search--surface .search-options-trigger { color: var(--color-text-secondary); }
.unified-search--surface .search-modes button[aria-pressed="true"] { color: var(--color-accent); background: var(--color-accent-soft); box-shadow: none; }
.unified-search--surface .search-modes button:hover, .unified-search--surface .search-options-trigger:hover { background: var(--color-surface-hover); color: var(--color-text-primary); }
.unified-search--surface input::placeholder, .unified-search--surface textarea::placeholder { color: var(--color-text-tertiary); }
.unified-search--surface .search-options { position: static; margin-top: var(--space-3); color: var(--color-text-primary); background: var(--color-surface-muted); border-color: var(--color-border); backdrop-filter: none; }
.unified-search--surface .search-options span { color: var(--color-text-secondary); }
.unified-search--sidebar .search-modes { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--space-1); padding: var(--space-1); border-radius: var(--radius-control); background: var(--color-surface-muted); margin-bottom: var(--space-4); }
.unified-search--sidebar .search-modes button { min-width: 0; min-height: calc(var(--space-10) + var(--space-1)); padding-inline: var(--space-1); gap: var(--space-1); border-radius: var(--radius-control); font-size: var(--font-size-small); white-space: nowrap; }
.unified-search--sidebar .search-modes svg { width: var(--space-4); height: var(--space-4); }
.unified-search--sidebar .search-body { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--space-3); padding: 0; border: 0; border-radius: 0; box-shadow: none; }
.unified-search--sidebar .search-body:focus-within { outline: none; }
.unified-search--sidebar .search-field-label { position: static; padding: 0; font-size: var(--font-size-small); background: var(--color-transparent); }
.unified-search--sidebar .search-leading { display: none; }
.unified-search--sidebar .search-body input, .unified-search--sidebar .search-body textarea { width: 100%; min-width: 0; border: var(--border-width) solid var(--color-border); border-radius: var(--radius-control); background: var(--color-surface-muted); font-size: var(--font-size-small); }
.unified-search--sidebar .search-body input:focus-visible, .unified-search--sidebar .search-body textarea:focus-visible { outline: var(--focus-outline-width) solid var(--color-accent); outline-offset: var(--focus-outline-offset); }
.unified-search--sidebar .search-submit { width: 100%; border-radius: var(--radius-control); }
.unified-search--sidebar .search-ai-footer { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--space-2); }
.unified-search--sidebar .search-options-trigger { padding-inline: 0; min-height: calc(var(--space-10) + var(--space-1)); }
.unified-search--sidebar .search-shortcut { display: none; }
.unified-search--sidebar .search-options { flex-wrap: wrap; gap: var(--space-2); padding: var(--space-3); box-shadow: none; }
.unified-search--sidebar .search-options input { width: var(--space-16); }
.unified-search--sidebar .search-options span { width: 100%; margin-left: 0; }
@media (prefers-reduced-motion: reduce) {
  .unified-search button:active:not(:disabled) { transform: none; opacity: 0.9; }
}
</style>
