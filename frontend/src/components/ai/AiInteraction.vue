<script setup lang="ts">
import { computed, ref, watch } from "vue"
import type { AgentRun, AgentResumeRequest } from "@/request/generated"
import { confirmationModel, textInputModel, selectionModel } from "@/services/ai-business"
import MarkdownRenderer from "@/components/article/MarkdownRenderer.vue"
const props = defineProps<{ interaction: NonNullable<AgentRun["pendingInteraction"]>; disabled: boolean; draft?: unknown }>()
const emit = defineEmits<{ submit: [body: AgentResumeRequest]; draft: [value: unknown] }>()
const text = ref("")
const selected = ref<number[]>([])
watch(() => props.interaction.interactionId, () => {
  text.value = typeof props.draft === "string" ? props.draft : ""
  selected.value = Array.isArray(props.draft) ? props.draft.filter((id): id is number => typeof id === "number" && Number.isSafeInteger(id) && id > 0) : []
}, { immediate: true })
const confirmation = computed(() => props.interaction.type === "confirmation" ? confirmationModel.safeParse(props.interaction.payload) : undefined)
const textInput = computed(() => props.interaction.type === "textInput" ? textInputModel.safeParse(props.interaction.payload) : undefined)
const selection = computed(() => props.interaction.type === "selection" ? selectionModel.safeParse(props.interaction.payload) : undefined)
const validText = computed(() => textInput.value?.success && text.value.trim().length >= textInput.value.data.min_length && text.value.trim().length <= textInput.value.data.max_length)
const validSelection = computed(() => selection.value?.success && selected.value.length >= selection.value.data.min_selections && selected.value.length <= selection.value.data.max_selections && new Set(selected.value).size === selected.value.length && selected.value.every((id) => selection.value?.success && selection.value.data.options.some((option) => option.article_id === id)))
function decide(decision: string, payload?: unknown): void {
  emit("submit", { interactionId: props.interaction.interactionId, decision, ...(payload === undefined ? {} : { payload }) })
}
</script>
<template>
  <section class="ai-panel" aria-label="待处理交互">
    <h2>需要你的回应</h2>
    <template v-if="confirmation?.success">
      <h3>{{ confirmation.data.title }}</h3><MarkdownRenderer :markdown="confirmation.data.outline" />
      <p>确认只允许继续本次生成，不代表保存或发布。</p>
      <div class="flex gap-4"><button type="button" :disabled="disabled" @click="decide('approve')">确认继续</button><button type="button" :disabled="disabled" @click="decide('reject')">拒绝草稿</button></div>
    </template>
    <form v-else-if="textInput?.success" @submit.prevent="decide('submit', text.trim())">
      <h3>{{ textInput.data.title }}</h3><p>{{ textInput.data.prompt }}</p>
      <label>补充写作要求<textarea v-model="text" rows="7" :maxlength="textInput.data.max_length" :disabled="disabled" @input="emit('draft', text)" /></label>
      <p>{{ text.trim().length }} / {{ textInput.data.max_length }}</p>
      <button type="submit" :disabled="disabled || !validText">提交补充要求</button>
    </form>
    <form v-else-if="selection?.success" @submit.prevent="decide('select', { article_ids: [...selected] })">
      <h3>{{ selection.data.title }}</h3><p>{{ selection.data.prompt }}</p>
      <p>可全不选，明确不使用参考继续。选中文章会提供正文片段，而非宣称读取全文。</p>
      <label v-for="option in selection.data.options" :key="option.article_id" class="ai-reference flex items-start gap-4">
        <input v-model="selected" type="checkbox" :value="option.article_id" :disabled="disabled" @change="emit('draft', [...selected])" />
        <span><strong>{{ option.title }}</strong><span class="block">{{ option.status }} · #{{ option.article_id }}</span><span class="block">{{ option.digest }}</span></span>
      </label>
      <button type="submit" :disabled="disabled || !validSelection">提交参考选择（{{ selected.length }} 篇）</button>
    </form>
    <p v-else role="alert">交互内容无法安全呈现，请查询最新状态或取消任务；不会猜测提交内容。</p>
  </section>
</template>
