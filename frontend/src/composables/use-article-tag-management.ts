import { computed, ref, type ComputedRef } from "vue"
import { HttpRequestError } from "@/request/http-error"
import { TAXONOMY_MUTATION_DOMAIN } from "@/services/mutation-guard"
import type { AuthorRuntime } from "@/services/author-runtime"
import type { AuthorTag } from "@/services/author-contracts"
import { useEditorDraftStore } from "@/stores/editor-draft"
import { useUiStore } from "@/stores/ui"

/** 标签写入反馈复用的稳定标题。 */
const TAG_TOAST_TITLE = {
  created: "标签已创建",
  updated: "标签已修改",
  deleted: "标签已删除",
  updateFailed: "修改标签失败",
  deleteFailed: "删除标签失败",
} as const

/** 文章标签管理所需的运行时与共享写入锁。 */
export type ArticleTagManagementOptions = {
  /** 提供标签仓储与写入守卫的作者运行时。 */ readonly runtime: AuthorRuntime
  /** 承载文章标签选择的编辑草稿。 */ readonly draftStore: ReturnType<typeof useEditorDraftStore>
  /** 展示标签写入反馈的全局界面状态。 */ readonly ui: ReturnType<typeof useUiStore>
  /** 分类写入期间阻止标签写入重叠的共享状态。 */ readonly categoryMutationBusy: ComputedRef<boolean>
}

/** 将标签仓储异常转换为不泄露实现细节的中文说明。 */
function tagMutationFailureDescription(error: unknown): string {
  return error instanceof HttpRequestError ? "标签服务请求失败，请重试" : "操作未完成，请重试"
}

/** 编排文章编辑器内标签创建、直接删除与草稿选择。 */
export function useArticleTagManagement(options: ArticleTagManagementOptions) {
  /** 当前可选择和删除的版本化标签。 */
  const tags = ref<readonly AuthorTag[]>([])
  /** 新增标签弹窗是否打开。 */
  const createTagModalOpen = ref(false)
  /** 新增标签原始名称。 */
  const tagName = ref("")
  /** 去除首尾空白后的标签请求名称。 */
  const trimmedTagName = computed(() => tagName.value.trim())
  /** 标签创建、修改与刷新协调是否正在执行。 */
  const createTagPending = ref(false)
  /** 待编辑的完整版本化标签。 */
  const updateTagTarget = ref<AuthorTag | null>(null)
  /** 修改标签原始输入。 */
  const updateTagName = ref("")
  /** 修改标签去除首尾空白后的请求名称。 */
  const trimmedUpdateTagName = computed(() => updateTagName.value.trim())
  /** 修改标签输入是否有效。 */
  const updateTagValid = computed(() => trimmedUpdateTagName.value.length > 0 && trimmedUpdateTagName.value.length <= 60 && updateTagTarget.value?.name !== trimmedUpdateTagName.value)
  /** 标签修改请求是否正在执行。 */
  const updateTagPending = ref(false)
  /** 标签删除与刷新协调是否正在执行。 */
  const deleteTagPending = ref(false)
  /** 标签写入及其列表协调阶段共享的状态。 */
  const tagMutationPending = computed(() => createTagPending.value || updateTagPending.value || deleteTagPending.value)
  /** 分类或标签任一写入期间均阻止标签入口。 */
  const taxonomyMutationBusy = computed(() => options.categoryMutationBusy.value || tagMutationPending.value)

  /** 仅刷新版本化标签，不改变草稿选择。 */
  async function refreshTags(): Promise<void> {
    tags.value = await Promise.resolve().then(() => options.runtime.taxonomy.listTags())
  }

  /** 写入受控标签选择；协调期间忽略已经排队的事件。 */
  function updateTagSelection(tagIds: readonly number[]): void {
    if (taxonomyMutationBusy.value) return
    options.draftStore.update({ ...options.draftStore.draft, tagIds })
  }

  /** 打开新增标签弹窗，写入协调期间保持静默。 */
  function openCreateTagModal(): void {
    if (taxonomyMutationBusy.value) return
    createTagModalOpen.value = true
  }

  /** 关闭新增标签弹窗并仅清空名称，不改变现有标签选择。 */
  function closeCreateTagModal(): void {
    if (tagMutationPending.value) return
    createTagModalOpen.value = false
    tagName.value = ""
  }

  /** 通过共享守卫创建标签，并以创建结果抵抗失败或陈旧刷新。 */
  async function createTag(): Promise<void> {
    if (taxonomyMutationBusy.value || trimmedTagName.value === "") return
    createTagPending.value = true
    const result = await Promise.resolve().then(() => options.runtime.guard.execute(TAXONOMY_MUTATION_DOMAIN, () => options.runtime.taxonomy.createTag(trimmedTagName.value))).then(
      (value) => value,
      (error: unknown) => {
        createTagPending.value = false
        options.ui.showToast("error", "创建标签失败", tagMutationFailureDescription(error))
        return null
      },
    )
    if (result === null) return
    if (!result.ok) {
      createTagPending.value = false
      options.ui.showToast("error", "创建标签被阻止", result.error.reason)
      return
    }

    const created = result.value
    createTagModalOpen.value = false
    tagName.value = ""
    const refreshed = await refreshTags().then(
      () => true,
      () => {
        options.ui.showToast("warning", TAG_TOAST_TITLE.created, "标签列表刷新失败，已保留新标签供当前编辑使用")
        return false
      },
    )
    // 创建结果按标识覆盖陈旧列表，并在协调锁释放前追加到草稿选择。
    tags.value = [...tags.value.filter((tag) => tag.id !== created.id), created]
    const selectedTagIds = options.draftStore.draft.tagIds
    const tagIds = selectedTagIds.includes(created.id) ? selectedTagIds : [...selectedTagIds, created.id]
    options.draftStore.update({ ...options.draftStore.draft, tagIds })
    createTagPending.value = false
    if (refreshed) options.ui.showToast("success", TAG_TOAST_TITLE.created)
  }
  /** 打开标签修改弹框，协调期间拒绝重叠入口。 */
  function requestUpdateTag(target: AuthorTag): void {
    if (taxonomyMutationBusy.value) return
    updateTagTarget.value = target
    updateTagName.value = target.name
  }

  /** 关闭未提交的标签修改弹框。 */
  function closeUpdateTag(): void {
    if (updateTagPending.value) return
    updateTagTarget.value = null
    updateTagName.value = ""
  }

  /** 通过共享守卫修改标签，并处理版本冲突后的最新资源。 */
  async function updateTag(): Promise<void> {
    const target = updateTagTarget.value
    if (target === null || taxonomyMutationBusy.value || !updateTagValid.value) return
    updateTagPending.value = true
    const result = await Promise.resolve().then(() => options.runtime.guard.execute(TAXONOMY_MUTATION_DOMAIN, () => options.runtime.taxonomy.updateTag({ id: target.id, version: target.version, changes: { name: trimmedUpdateTagName.value } }))).then(
      (value) => value,
      async (error: unknown) => {
        if (error instanceof HttpRequestError && error.status === 409) {
          updateTagPending.value = false
          options.ui.showToast("error", TAG_TOAST_TITLE.updateFailed, "标签名称已存在")
          return null
        }
        if (error instanceof HttpRequestError && (error.status === 404 || error.status === 412)) {
          const refreshed = await refreshTags().then(() => true, () => false)
          if (!refreshed) {
            updateTagPending.value = false
            options.ui.showToast("error", "标签状态刷新失败", "无法获取标签最新状态，请稍后重试修改")
            return null
          }
          const refreshedTarget = tags.value.find((tag) => tag.id === target.id)
          if (refreshedTarget === undefined) {
            removeTag(target.id)
            updateTagTarget.value = null
            updateTagName.value = ""
            updateTagPending.value = false
            options.ui.showToast("info", "标签状态已更新", "该标签已不存在，当前选择已同步")
            return null
          }
          updateTagTarget.value = refreshedTarget
          updateTagName.value = refreshedTarget.name
          updateTagPending.value = false
          options.ui.showToast("warning", "标签版本已更新", "标签已被其他操作修改，请确认最新名称后再保存")
          return null
        }
        updateTagPending.value = false
        options.ui.showToast("error", TAG_TOAST_TITLE.updateFailed, tagMutationFailureDescription(error))
        return null
      },
    )
    if (result === null) return
    if (!result.ok) {
      updateTagPending.value = false
      options.ui.showToast("error", "修改标签被阻止", result.error.reason)
      return
    }
    const updated = result.value
    tags.value = [...tags.value.filter((tag) => tag.id !== updated.id), updated]
    updateTagTarget.value = null
    updateTagName.value = ""
    updateTagPending.value = false
    options.ui.showToast("success", TAG_TOAST_TITLE.updated)
  }


  /** 从草稿选择和当前列表中收敛一个已不存在的标签。 */
  function removeTag(tagId: number): void {
    const tagIds = options.draftStore.draft.tagIds.filter((id) => id !== tagId)
    options.draftStore.update({ ...options.draftStore.draft, tagIds })
    tags.value = tags.value.filter((tag) => tag.id !== tagId)
  }

  /** 处理标签版本冲突刷新，并要求基于最新资源再次点击删除。 */
  async function reconcileTagVersionConflict(target: AuthorTag): Promise<void> {
    const refreshed = await refreshTags().then(
      () => true,
      () => false,
    )
    if (!refreshed) {
      deleteTagPending.value = false
      options.ui.showToast("error", "标签状态刷新失败", "无法获取标签最新状态，请稍后重试删除")
      return
    }
    const refreshedTarget = tags.value.find((tag) => tag.id === target.id)
    if (refreshedTarget === undefined) {
      removeTag(target.id)
      deleteTagPending.value = false
      options.ui.showToast("info", "标签状态已更新", "该标签已不存在，当前选项已同步")
      return
    }
    deleteTagPending.value = false
    options.ui.showToast("warning", "标签版本已更新", "标签已发生变化，请再次点击删除")
  }

  /** 立即通过版本化请求删除标签，不打开删除确认弹窗。 */
  async function requestDeleteTag(target: AuthorTag): Promise<void> {
    if (taxonomyMutationBusy.value) return
    deleteTagPending.value = true
    const result = await Promise.resolve().then(() => options.runtime.guard.execute(TAXONOMY_MUTATION_DOMAIN, () => options.runtime.taxonomy.deleteTag({ id: target.id, version: target.version }))).then(
      (value) => value,
      async (error: unknown) => {
        if (error instanceof HttpRequestError && error.status === 409) {
          deleteTagPending.value = false
          options.ui.showToast("error", TAG_TOAST_TITLE.deleteFailed, "该标签仍被文章引用，无法删除")
          return null
        }
        if (error instanceof HttpRequestError && error.status === 412) {
          await reconcileTagVersionConflict(target)
          return null
        }
        deleteTagPending.value = false
        options.ui.showToast("error", TAG_TOAST_TITLE.deleteFailed, tagMutationFailureDescription(error))
        return null
      },
    )
    if (result === null) return
    if (!result.ok) {
      deleteTagPending.value = false
      options.ui.showToast("error", "删除标签被阻止", result.error.reason)
      return
    }

    // 删除成功后先移除草稿选择，再以已删标识过滤失败或陈旧的刷新结果。
    removeTag(target.id)
    const refreshed = await refreshTags().then(
      () => true,
      () => {
        options.ui.showToast("warning", TAG_TOAST_TITLE.deleted, "标签列表刷新失败，已从当前选项中移除该标签")
        return false
      },
    )
    tags.value = tags.value.filter((tag) => tag.id !== target.id)
    deleteTagPending.value = false
    if (refreshed) options.ui.showToast("success", TAG_TOAST_TITLE.deleted)
  }

  return {
    tags,
    createTagModalOpen,
    tagName,
    trimmedTagName,
    createTagPending,
    updateTagTarget,
    updateTagName,
    trimmedUpdateTagName,
    updateTagValid,
    updateTagPending,
    deleteTagPending,
    tagMutationPending,
    refreshTags,
    updateTagSelection,
    openCreateTagModal,
    closeCreateTagModal,
    createTag,
    requestUpdateTag,
    closeUpdateTag,
    updateTag,
    requestDeleteTag,
  }
}
