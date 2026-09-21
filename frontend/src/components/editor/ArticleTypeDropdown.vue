<script setup lang="ts">
import { ChevronDownIcon, PencilSquareIcon, TrashIcon } from "@heroicons/vue/24/outline"
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from "vue"
import type { AuthorArticleType } from "@/services/author-contracts"

/** 下拉框输入属性。 */
type Props = {
  /** 父级控制的当前文章分类标识。 */ readonly articleTypeId: number
  /** 可选择、修改和删除的版本化文章分类资源。 */ readonly articleTypes: readonly AuthorArticleType[]
  /** 父级控制的分类变更锁。 */ readonly disabled?: boolean
}

/** 固定在列表首位的创建操作文案。 */
const CREATE_LABEL = "新增分类"
/** 连续输入被视为同一段检索文本的间隔。 */
const TYPEAHEAD_RESET_MS = 700

const props = withDefaults(defineProps<Props>(), { disabled: false })
/** 文章分类选择、创建请求与版本化删除请求。 */
const emit = defineEmits<{
  "update:articleTypeId": [value: number]
  "request-create-article-type": []
  "request-update-article-type": [articleType: AuthorArticleType]
  "request-delete-article-type": [articleType: AuthorArticleType]
}>()

/** 当前组件实例的稳定标识前缀。 */
const instanceId = useId()
/** 可见标签的稳定标识。 */
const labelId = `${instanceId}-label`
/** 列表框的稳定标识。 */
const listboxId = `${instanceId}-listbox`
/** 包含触发器和浮层的组件根节点。 */
const rootElement = ref<HTMLElement | null>(null)
/** 关闭后需要恢复焦点的组合框触发器。 */
const triggerElement = ref<HTMLButtonElement | null>(null)
/** 浮层当前是否打开。 */
const isOpen = ref(false)
/** 由组合框 aria-activedescendant 指向的虚拟活动选项位置。 */
const activeIndex = ref(0)
/** 实用前缀检索当前累积的文本。 */
let typeaheadBuffer = ""
/** 前缀检索重置计时器。 */
let typeaheadTimer: number | undefined

/** 列表中的稳定选项文本，首项始终为创建操作。 */
const optionLabels = computed<readonly string[]>(() => [CREATE_LABEL, ...props.articleTypes.map((articleType) => articleType.name)])
/** 当前受控分类资源。 */
const selectedArticleType = computed(() => props.articleTypes.find((articleType) => articleType.id === props.articleTypeId))
/** 组合框显示的受控文本。 */
const selectedLabel = computed(() => selectedArticleType.value?.name ?? "请选择分类")
/** 当前虚拟活动选项的 DOM 标识。 */
const activeOptionId = computed(() => `${instanceId}-option-${activeIndex.value}`)

/** 返回指定位置选项的稳定 DOM 标识。 */
const optionId = (index: number): string => `${instanceId}-option-${index}`

/** 清除前缀检索状态和计时器。 */
const resetTypeahead = (): void => {
  typeaheadBuffer = ""
  if (typeaheadTimer !== undefined) window.clearTimeout(typeaheadTimer)
  typeaheadTimer = undefined
}

/** 打开列表并设置虚拟活动项，真实焦点始终留在组合框。 */
const openDropdown = (preferredIndex?: number): void => {
  if (props.disabled) return
  const selectedIndex = props.articleTypes.findIndex((articleType) => articleType.id === props.articleTypeId)
  activeIndex.value = preferredIndex ?? (selectedIndex >= 0 ? selectedIndex + 1 : 0)
  isOpen.value = true
}

/** 关闭列表，并按交互来源选择是否把焦点还给触发器。 */
const closeDropdown = async (restoreFocus: boolean): Promise<void> => {
  isOpen.value = false
  resetTypeahead()
  if (!restoreFocus) return
  await nextTick()
  triggerElement.value?.focus()
}

/** 切换鼠标触发的列表状态。 */
const toggleDropdown = async (): Promise<void> => {
  if (props.disabled) return
  if (isOpen.value) {
    await closeDropdown(false)
    return
  }
  openDropdown()
}

/** 更新虚拟活动位置，不移动实际键盘焦点。 */
const setActiveIndex = (index: number): void => {
  activeIndex.value = index
}

/** 按方向循环移动虚拟活动选项。 */
const moveActiveIndex = (offset: number): void => {
  const optionCount = optionLabels.value.length
  setActiveIndex((activeIndex.value + offset + optionCount) % optionCount)
}

/** 激活创建操作或真实分类，受控值只通过事件交给父级。 */
const activateOption = async (index: number): Promise<void> => {
  if (props.disabled) return
  if (index === 0) {
    emit("request-create-article-type")
    await closeDropdown(true)
    return
  }
  const articleType = props.articleTypes[index - 1]
  if (articleType === undefined) return
  emit("update:articleTypeId", articleType.id)
  await closeDropdown(true)
}

/** 发出版本化删除请求，不改变选择或列表开关状态。 */
/** 发出版本化修改请求，不改变选择或列表开关状态。 */
const requestUpdate = (articleType: AuthorArticleType): void => {
  if (props.disabled) return
  emit("request-update-article-type", articleType)
}
const requestDelete = (articleType: AuthorArticleType): void => {
  if (props.disabled) return
  emit("request-delete-article-type", articleType)
}

/** 根据连续字符输入寻找下一项前缀匹配。 */
const findTypeaheadMatch = (key: string): number | null => {
  if (typeaheadTimer !== undefined) window.clearTimeout(typeaheadTimer)
  typeaheadBuffer += key.toLocaleLowerCase("zh-CN")
  typeaheadTimer = window.setTimeout(resetTypeahead, TYPEAHEAD_RESET_MS)
  const optionCount = optionLabels.value.length
  for (let offset = 1; offset <= optionCount; offset += 1) {
    const candidateIndex = (activeIndex.value + offset) % optionCount
    if (optionLabels.value[candidateIndex]?.toLocaleLowerCase("zh-CN").startsWith(typeaheadBuffer)) return candidateIndex
  }
  return null
}

/** 处理组合框上的虚拟导航、激活、关闭与前缀检索。 */
const handleKeydown = async (event: KeyboardEvent): Promise<void> => {
  if (props.disabled) return
  switch (event.key) {
    case "ArrowDown":
      event.preventDefault()
      if (isOpen.value) moveActiveIndex(1)
      else openDropdown()
      return
    case "ArrowUp":
      event.preventDefault()
      if (isOpen.value) moveActiveIndex(-1)
      else openDropdown()
      return
    case "Home":
      event.preventDefault()
      if (isOpen.value) setActiveIndex(0)
      else openDropdown(0)
      return
    case "End":
      event.preventDefault()
      if (isOpen.value) setActiveIndex(optionLabels.value.length - 1)
      else openDropdown(optionLabels.value.length - 1)
      return
    case "Enter":
    case " ":
      event.preventDefault()
      if (isOpen.value) await activateOption(activeIndex.value)
      else openDropdown()
      return
    case "Escape":
      if (isOpen.value) {
        event.preventDefault()
        event.stopPropagation()
        await closeDropdown(true)
      }
      return
    default:
      if (event.ctrlKey || event.metaKey || event.altKey || event.key.length !== 1) return
      event.preventDefault()
      const matchIndex = findTypeaheadMatch(event.key)
      if (matchIndex === null) return
      if (isOpen.value) setActiveIndex(matchIndex)
      else openDropdown(matchIndex)
  }
}

/** 弹层原生控件按 Escape 时关闭弹层并恢复组合框焦点。 */
const handlePopupKeydown = async (event: KeyboardEvent): Promise<void> => {
  if (event.key !== "Escape") return
  event.preventDefault()
  event.stopPropagation()
  await closeDropdown(true)
}

/** 指针或键盘焦点离开组件时关闭浮层，不抢回用户的新焦点。 */
const closeFromOutside = (event: Event): void => {
  if (!isOpen.value || !(event.target instanceof Node) || rootElement.value?.contains(event.target)) return
  void closeDropdown(false)
}

/** 变更锁启用时立即关闭弹层，禁用触发器不再接收恢复焦点。 */
watch(() => props.disabled, (disabled) => {
  if (disabled && isOpen.value) void closeDropdown(false)
})

onMounted(() => {
  document.addEventListener("pointerdown", closeFromOutside)
  document.addEventListener("focusin", closeFromOutside)
})
onBeforeUnmount(() => {
  document.removeEventListener("pointerdown", closeFromOutside)
  document.removeEventListener("focusin", closeFromOutside)
  resetTypeahead()
})
</script>

<template>
  <div ref="rootElement" class="grid gap-2 text-sm font-medium">
    <span :id="labelId">分类</span>
    <div class="relative">
      <button
        ref="triggerElement"
        type="button"
        role="combobox"
        class="flex min-h-11 w-full items-center justify-between gap-3 rounded-[var(--radius-control)] border border-border bg-surface px-3 text-left text-text-primary transition-colors duration-[var(--duration-fast)] hover:bg-surface-hover disabled:cursor-not-allowed disabled:opacity-60"
        :disabled="disabled"
        :aria-labelledby="labelId"
        :aria-expanded="isOpen"
        aria-haspopup="listbox"
        :aria-controls="listboxId"
        :aria-activedescendant="isOpen ? activeOptionId : undefined"
        @click="toggleDropdown"
        @keydown="handleKeydown"
      >
        <span class="truncate">{{ selectedLabel }}</span>
        <ChevronDownIcon class="size-5 shrink-0 text-text-secondary transition-transform duration-[var(--duration-fast)]" :class="isOpen ? 'rotate-180' : ''" aria-hidden="true" />
      </button>

      <div v-if="isOpen" class="absolute inset-x-0 top-full z-[var(--layer-floating-control)] mt-2 overflow-hidden rounded-[var(--radius-control)] border border-border bg-surface shadow-dialog" @keydown="handlePopupKeydown">
        <div class="grid grid-cols-[minmax(0,1fr)_var(--space-24)]">
          <div :id="listboxId" role="listbox" :aria-labelledby="labelId" class="min-w-0 py-1">
            <button
              :id="optionId(0)"
              type="button"
              role="option"
              class="flex min-h-11 w-full items-center px-3 text-left transition-colors duration-[var(--duration-fast)] disabled:cursor-not-allowed disabled:opacity-60"
              :class="activeIndex === 0 ? 'bg-surface-hover text-text-primary' : 'text-accent hover:bg-accent-soft'"
              :disabled="disabled"
              :aria-selected="false"
              tabindex="-1"
              @click="activateOption(0)"
            >
              {{ CREATE_LABEL }}
            </button>
            <button
              v-for="(articleType, index) in articleTypes"
              :id="optionId(index + 1)"
              :key="articleType.id"
              type="button"
              role="option"
              class="flex min-h-11 w-full items-center truncate px-3 text-left transition-colors duration-[var(--duration-fast)] disabled:cursor-not-allowed disabled:opacity-60"
              :class="activeIndex === index + 1 ? 'bg-surface-hover text-text-primary' : articleType.id === articleTypeId ? 'bg-accent-soft text-accent' : 'text-text-primary hover:bg-surface-hover'"
              :disabled="disabled"
              :aria-selected="articleType.id === articleTypeId"
              tabindex="-1"
              @click="activateOption(index + 1)"
            >
              {{ articleType.name }}
            </button>
          </div>

          <div role="group" class="grid grid-cols-2 py-1" aria-label="分类修改与删除操作">
            <div class="col-span-2 min-h-11" aria-hidden="true" />
            <template v-for="articleType in articleTypes" :key="articleType.id">
              <button
                type="button"
                class="flex min-h-11 w-full items-center justify-center text-text-secondary transition-colors duration-[var(--duration-fast)] hover:bg-accent-soft hover:text-accent disabled:cursor-not-allowed disabled:opacity-60"
                :disabled="disabled"
                :aria-label="`修改分类“${articleType.name}”`"
                @click.stop="requestUpdate(articleType)"
              >
                <PencilSquareIcon class="size-5" aria-hidden="true" />
              </button>
              <button
                type="button"
                class="flex min-h-11 w-full items-center justify-center text-text-secondary transition-colors duration-[var(--duration-fast)] hover:bg-error-soft hover:text-error disabled:cursor-not-allowed disabled:opacity-60"
                :disabled="disabled"
                :aria-label="`删除分类“${articleType.name}”`"
                @click.stop="requestDelete(articleType)"
              >
                <TrashIcon class="size-5" aria-hidden="true" />
              </button>
            </template>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>