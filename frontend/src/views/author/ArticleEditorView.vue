<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue"
import { useRoute, useRouter } from "vue-router"
import ArticleSettings from "@/components/editor/ArticleSettings.vue"
import RichMarkdownEditor from "@/components/editor/RichMarkdownEditor.vue"
import MarkdownRenderer from "@/components/article/MarkdownRenderer.vue"
import AppModal from "@/components/shared/AppModal.vue"
import AppToast from "@/components/shared/AppToast.vue"
import { useApiServices } from "@/request/api-services"
import { HttpRequestError } from "@/request/http-error"
import { TAXONOMY_MUTATION_DOMAIN } from "@/services/mutation-guard"
import { createAuthorRuntime, type AuthorRuntime } from "@/services/author-runtime"
import { useAuthSession } from "@/services/auth-session"
import type { AuthorArticleType } from "@/services/author-contracts"
import { useArticleTagManagement } from "@/composables/use-article-tag-management"
import { createImageLifecycle } from "@/services/image-lifecycle"
import { useEditorDraftStore } from "@/stores/editor-draft"
import { useUiStore } from "@/stores/ui"

/** 测试可注入的作者运行时。 */
const props = defineProps<{ readonly runtime?: AuthorRuntime }>()
/** 当前作者运行时；测试可注入隔离 Fake，路由页面使用真实 API 与认证状态。 */
const runtime = props.runtime ?? createAuthorRuntime(useApiServices(), useAuthSession().currentState)
/** 当前编辑草稿。 */ const draftStore = useEditorDraftStore()
/** 全局反馈。 */ const ui = useUiStore()
/** 路由参数。 */
const route = useRoute()
/** 创建成功后进入服务端文章编辑地址。 */
const router = useRouter()
/** 图片生命周期。 */ const images = createImageLifecycle(runtime.articles, runtime.guard)
/** 带并发版本的分类选项。 */ const articleTypes = ref<readonly AuthorArticleType[]>([])
/** 初始化状态，失败时保留可重试分支。 */ const initialization = ref<"loading" | "ready" | "error">("loading")
/** 新增分类弹窗是否打开。 */ const createArticleTypeModalOpen = ref(false)
/** 新增分类原始输入。 */ const articleTypeName = ref("")
/** 新增分类去除首尾空白后的请求名称。 */ const trimmedArticleTypeName = computed(() => articleTypeName.value.trim())
/** 待编辑的完整版本化分类。 */
const updateArticleTypeTarget = ref<AuthorArticleType | null>(null)
/** 修改分类原始输入。 */
const updateArticleTypeName = ref("")
/** 修改分类去除首尾空白后的请求名称。 */
const trimmedUpdateArticleTypeName = computed(() => updateArticleTypeName.value.trim())
/** 分类修改请求是否正在执行。 */
const updateArticleTypePending = ref(false)
/** 分类修改输入是否有效。 */
const updateArticleTypeValid = computed(() => trimmedUpdateArticleTypeName.value.length > 0 && trimmedUpdateArticleTypeName.value.length <= 60 && updateArticleTypeTarget.value?.name !== trimmedUpdateArticleTypeName.value)
/** 待确认删除的完整版本化分类。 */
const deleteArticleTypeTarget = ref<AuthorArticleType | null>(null)
/** 新增分类请求是否正在执行。 */
const createArticleTypePending = ref(false)
/** 删除分类请求是否正在执行。 */
const deleteArticleTypePending = ref(false)
/** 新增、修改、删除及其协调阶段共享的交互锁。 */
const categoryMutationBusy = computed(() => createArticleTypePending.value || updateArticleTypePending.value || deleteArticleTypePending.value)
/** 标签创建、删除、刷新和草稿选择状态。 */
const { tags, createTagModalOpen, tagName, trimmedTagName, createTagPending, updateTagTarget, updateTagName, updateTagValid, updateTagPending, tagMutationPending, refreshTags, updateTagSelection, openCreateTagModal, closeCreateTagModal, createTag, requestUpdateTag, closeUpdateTag, updateTag, requestDeleteTag } = useArticleTagManagement({ runtime, draftStore, ui, categoryMutationBusy })
/** 分类或标签任一写入及其协调阶段共享的交互锁。 */ const taxonomyMutationBusy = computed(() => categoryMutationBusy.value || tagMutationPending.value)

/** 将仓储异常归一化为不泄露实现细节的中文重试说明。 */
function mutationFailureDescription(error: unknown): string {
  return error instanceof HttpRequestError ? "分类服务请求失败，请重试" : "操作未完成，请重试"
}

/** 仅刷新文章分类，不触碰标签、正文或草稿。 */
async function refreshArticleTypes(): Promise<void> {
  articleTypes.value = await Promise.resolve().then(() => runtime.taxonomy.listArticleTypes())
}

/** 初始化创建或编辑模式。 */
async function initialize(): Promise<void> {
  initialization.value = "loading"
  const id = Number(route.params["id"])
  const articleTypesRequest = refreshArticleTypes()
  const tagsRequest = refreshTags()
  const articleRequest = Number.isInteger(id) && id > 0 ? Promise.resolve().then(() => runtime.articles.detail(id)) : Promise.resolve(null)
  await Promise.all([articleTypesRequest, tagsRequest, articleRequest]).then(
    ([, , article]) => {
      if (article === null) draftStore.reset(); else draftStore.load(article)
      initialization.value = "ready"
    },
    () => { initialization.value = "error"; ui.showToast("error", "写作台加载失败", "请重试载入草稿与分类") },
  )
}
onMounted(initialize)
/** 图片取消失败时告知用户由服务端 TTL 继续兜底。 */
const showImageCleanupFallback = (): void => { ui.showToast("error", "图片取消失败", "临时图片将由服务端过期清理") }
/** 离开未保存编辑器时清理临时图片。 */
onBeforeUnmount(() => { void images.cancel().then((cleaned) => { if (!cleaned) showImageCleanupFallback() }) })

/** 更新分类选择；协调期间忽略已经排队的下拉事件。 */
function updateArticleTypeSelection(articleTypeId: number): void {
  if (taxonomyMutationBusy.value) return
  draftStore.update({ ...draftStore.draft, articleTypeId })
}

/** 打开新增分类弹窗，协调期间拒绝重叠入口。 */
function openCreateArticleTypeModal(): void {
  if (taxonomyMutationBusy.value) return
  createArticleTypeModalOpen.value = true
}

/** 关闭未成功创建的弹窗，并仅清空分类选择与名称输入。 */
function closeCreateArticleTypeModal(): void {
  if (taxonomyMutationBusy.value) return
  createArticleTypeModalOpen.value = false
  articleTypeName.value = ""
  draftStore.update({ ...draftStore.draft, articleTypeId: 0 })
}

/** 通过共享分类守卫创建分类，并在刷新失败时保留返回资源。 */
async function createArticleType(): Promise<void> {
  if (taxonomyMutationBusy.value || trimmedArticleTypeName.value === "") return
  createArticleTypePending.value = true
  const result = await Promise.resolve().then(() => runtime.guard.execute(TAXONOMY_MUTATION_DOMAIN, () => runtime.taxonomy.createArticleType({ name: trimmedArticleTypeName.value, image: null, menu: articleTypes.value.length + 1 }))).then(
    (value) => value,
    (error: unknown) => {
      createArticleTypePending.value = false
      ui.showToast("error", "创建分类失败", mutationFailureDescription(error))
      return null
    },
  )
  if (result === null) return
  if (!result.ok) {
    createArticleTypePending.value = false
    ui.showToast("error", "创建分类被阻止", result.error.reason)
    return
  }

  // 写操作成功后关闭弹窗，但在分类刷新与结果协调完成前继续保持共享锁。
  const created = result.value
  createArticleTypeModalOpen.value = false
  articleTypeName.value = ""
  const refreshed = await refreshArticleTypes().then(
    () => true,
    () => {
      ui.showToast("warning", "分类已创建", "分类列表刷新失败，已保留新分类供当前编辑使用")
      return false
    },
  )
  // 无论刷新失败还是成功返回陈旧列表，都以创建结果按标识去重补齐当前选项。
  articleTypes.value = [...articleTypes.value.filter((articleType) => articleType.id !== created.id), created]
  draftStore.update({ ...draftStore.draft, articleTypeId: created.id })
  createArticleTypePending.value = false
  if (refreshed) ui.showToast("success", "分类已创建")
}

/** 打开分类修改弹框，协调期间拒绝重叠入口。 */
function requestUpdateArticleType(articleType: AuthorArticleType): void {
  if (taxonomyMutationBusy.value) return
  updateArticleTypeTarget.value = articleType
  updateArticleTypeName.value = articleType.name
}

/** 关闭未提交的分类修改弹框；协调期间忽略关闭操作。 */
function closeUpdateArticleTypeModal(): void {
  if (taxonomyMutationBusy.value) return
  updateArticleTypeTarget.value = null
  updateArticleTypeName.value = ""
}

/** 通过共享分类守卫修改分类，并处理冲突后的最新版本协调。 */
async function updateArticleType(): Promise<void> {
  const target = updateArticleTypeTarget.value
  if (target === null || taxonomyMutationBusy.value || !updateArticleTypeValid.value) return
  updateArticleTypePending.value = true
  const result = await Promise.resolve().then(() => runtime.guard.execute(TAXONOMY_MUTATION_DOMAIN, () => runtime.taxonomy.updateArticleType({ id: target.id, version: target.version, changes: { name: trimmedUpdateArticleTypeName.value } }))).then(
    (value) => value,
    async (error: unknown) => {
      if (error instanceof HttpRequestError && error.status === 409) {
        updateArticleTypePending.value = false
        ui.showToast("error", "分类修改失败", "分类名称已存在")
        return null
      }
      if (error instanceof HttpRequestError && (error.status === 404 || error.status === 412)) {
        const refreshed = await refreshArticleTypes().then(() => true, () => false)
        if (!refreshed) {
          updateArticleTypePending.value = false
          ui.showToast("error", "分类状态刷新失败", "无法获取分类最新状态，请稍后重试修改")
          return null
        }
        const refreshedTarget = articleTypes.value.find((articleType) => articleType.id === target.id)
        if (refreshedTarget === undefined) {
          updateArticleTypeTarget.value = null
          updateArticleTypeName.value = ""
          if (draftStore.draft.articleTypeId === target.id) draftStore.update({ ...draftStore.draft, articleTypeId: 0 })
          updateArticleTypePending.value = false
          ui.showToast("info", "分类状态已更新", "该分类已不存在，当前选择已同步")
          return null
        }
        updateArticleTypeTarget.value = refreshedTarget
        updateArticleTypeName.value = refreshedTarget.name
        updateArticleTypePending.value = false
        ui.showToast("warning", "分类版本已更新", "分类已被其他操作修改，请确认最新名称后再保存")
        return null
      }
      updateArticleTypePending.value = false
      ui.showToast("error", "修改分类失败", mutationFailureDescription(error))
      return null
    },
  )
  if (result === null) return
  if (!result.ok) {
    updateArticleTypePending.value = false
    ui.showToast("error", "修改分类被阻止", result.error.reason)
    return
  }
  const updated = result.value
  articleTypes.value = [...articleTypes.value.filter((articleType) => articleType.id !== updated.id), updated]
  updateArticleTypeTarget.value = null
  updateArticleTypeName.value = ""
  updateArticleTypePending.value = false
  ui.showToast("success", "分类已修改")
}

/** 打开分类删除确认框，协调期间拒绝重叠入口。 */
function requestDeleteArticleType(articleType: AuthorArticleType): void {
  if (taxonomyMutationBusy.value) return
  deleteArticleTypeTarget.value = articleType
}

/** 关闭未提交的删除确认框；协调期间忽略关闭操作。 */
function closeDeleteArticleTypeModal(): void {
  if (taxonomyMutationBusy.value) return
  deleteArticleTypeTarget.value = null
}

/** 通过共享分类守卫删除目标，并处理并发版本冲突与列表协调。 */
async function deleteArticleType(): Promise<void> {
  const target = deleteArticleTypeTarget.value
  if (target === null || taxonomyMutationBusy.value) return
  deleteArticleTypePending.value = true
  const result = await Promise.resolve().then(() => runtime.guard.execute(TAXONOMY_MUTATION_DOMAIN, () => runtime.taxonomy.deleteArticleType({ id: target.id, version: target.version }))).then(
    (value) => value,
    async (error: unknown) => {
      if (error instanceof HttpRequestError && (error.status === 409 || error.status === 412)) {
        const conflictRefreshed = await refreshArticleTypes().then(
          () => true,
          () => false,
        )
        if (!conflictRefreshed) {
          deleteArticleTypePending.value = false
          ui.showToast("error", "分类状态刷新失败", "无法获取分类最新状态，请稍后重试删除")
          return null
        }

        const refreshedTarget = articleTypes.value.find((articleType) => articleType.id === target.id)
        if (refreshedTarget === undefined) {
          // 冲突刷新确认目标已不存在，关闭弹窗并让本地选择与列表收敛。
          deleteArticleTypeTarget.value = null
          if (draftStore.draft.articleTypeId === target.id) draftStore.update({ ...draftStore.draft, articleTypeId: 0 })
          articleTypes.value = articleTypes.value.filter((articleType) => articleType.id !== target.id)
          deleteArticleTypePending.value = false
          ui.showToast("info", "分类状态已更新", "该分类已不存在，当前选项已同步")
          return null
        }

        // 使用刷新后的版本替换确认目标，要求用户基于最新状态重新确认。
        deleteArticleTypeTarget.value = refreshedTarget
        deleteArticleTypePending.value = false
        ui.showToast("warning", "分类版本已更新", "分类已发生变化，请再次确认删除")
        return null
      }

      deleteArticleTypePending.value = false
      ui.showToast("error", "删除分类失败", mutationFailureDescription(error))
      return null
    },
  )
  if (result === null) return
  if (!result.ok) {
    deleteArticleTypePending.value = false
    ui.showToast("error", "删除分类被阻止", result.error.reason)
    return
  }

  // 删除成功后才关闭确认框，并只在目标正被使用时清空选择。
  deleteArticleTypeTarget.value = null
  if (draftStore.draft.articleTypeId === target.id) draftStore.update({ ...draftStore.draft, articleTypeId: 0 })
  const refreshed = await refreshArticleTypes().then(
    () => true,
    () => {
      ui.showToast("warning", "分类已删除", "分类列表刷新失败，已从当前选项中移除该分类")
      return false
    },
  )
  articleTypes.value = articleTypes.value.filter((articleType) => articleType.id !== target.id)
  deleteArticleTypePending.value = false
  if (refreshed) ui.showToast("success", "分类已删除")
}

/** 保存文章并阻止重复提交。 */
async function save(): Promise<void> {
  if (!draftStore.beginSave()) return
  const id = Number(route.params["id"])
  const isEdit = Number.isInteger(id) && id > 0
  const version = draftStore.version
  if (isEdit && version === undefined) {
    draftStore.failSave()
    ui.showToast("error", "文章版本缺失", "请重新载入文章后再保存")
    return
  }
  const operation = isEdit
    ? () => runtime.articles.update({ id, version: version as number, changes: draftStore.toPatch() })
    : () => runtime.articles.create(draftStore.toCreate())
  const result = await Promise.resolve().then(() => runtime.guard.execute("article", operation)).then(
    (value) => value,
    async (error: unknown) => {
      draftStore.failSave()
      const cleaned = await images.cancel()
      if (!cleaned) showImageCleanupFallback()
      if (error instanceof HttpRequestError && error.status === 412) ui.showToast("error", "文章版本已变化", "服务端内容已被其他操作修改，草稿仍在，请重新载入后处理")
      else if (error instanceof HttpRequestError && error.status === 403) ui.showToast("error", "保存未授权", "当前作者身份没有保存权限")
      else if (error instanceof HttpRequestError && error.status === 404) ui.showToast("error", "文章不存在", "请重新载入文章后重试")
      else if (error instanceof HttpRequestError && error.status === 409) ui.showToast("error", "文章保存冲突", "请检查标题或文章链接名是否已经被使用")
      else if (error instanceof HttpRequestError && error.status === 428) ui.showToast("error", "文章版本缺失", "请重新载入文章后重试")
      else ui.showToast("error", "保存失败", "草稿仍在，请重试保存")
      return null
    },
  )
  if (result === null) return
  if (!result.ok) {
    draftStore.failSave()
    const cleaned = await images.cancel()
    if (!cleaned) showImageCleanupFallback()
    ui.showToast("error", "保存被阻止", result.error.reason)
    return
  }
  images.commit()
  draftStore.finishSave(result.value)
  if (!isEdit) await router.push(`/author/articles/${result.value.id}/edit`)
  ui.showToast("success", "文章已保存")
}
</script>

<template>
  <div class="grid gap-6"><AppToast /><header class="flex items-center justify-between"><div><p class="text-sm text-text-secondary">受保护的作者工作区</p><h1 class="font-display text-3xl font-bold">{{ route.params['id'] ? '编辑文章' : '新建文章' }}</h1></div><button type="button" data-testid="save-article" class="min-w-28 rounded-lg bg-accent px-5 py-3 font-semibold text-canvas disabled:opacity-60" :disabled="draftStore.saving || initialization !== 'ready'" :aria-busy="draftStore.saving" @click="save">{{ draftStore.saving ? '保存中…' : '保存文章' }}</button></header>
    <p v-if="initialization === 'loading'" role="status" class="rounded-[var(--radius-card)] border border-border bg-surface p-6">正在载入写作台…</p>
    <section v-else-if="initialization === 'error'" role="alert" class="rounded-[var(--radius-card)] border border-error bg-error-soft p-6"><h2 class="font-semibold text-error">写作台加载失败</h2><p class="mt-2 text-text-secondary">草稿与分类尚未载入，请重试。</p><button data-testid="retry-editor-init" class="mt-4 rounded-lg bg-accent px-4 py-3 font-semibold text-canvas" @click="initialize">重新载入</button></section>
    <template v-else>
      <ArticleSettings
        :title="draftStore.draft.title"
        :slug="draftStore.draft.slug"
        :article-type-id="draftStore.draft.articleTypeId"
        :tag-ids="draftStore.draft.tagIds"
        :article-types="articleTypes"
        :tags="tags"
        :disabled="taxonomyMutationBusy"
        @update:title="draftStore.updateTitle($event)"
        @update:slug="draftStore.updateSlug($event)"
        @update:article-type-id="updateArticleTypeSelection"
        @update:tag-ids="updateTagSelection"
        @request-create-article-type="openCreateArticleTypeModal"
        @request-update-article-type="requestUpdateArticleType"
        @request-delete-article-type="requestDeleteArticleType"
        @request-create-tag="openCreateTagModal"
        @request-update-tag="requestUpdateTag"
        @request-delete-tag="requestDeleteTag"
      />
      <RichMarkdownEditor :model-value="draftStore.draft.markdown" :images="images" @update:model-value="draftStore.update({ ...draftStore.draft, markdown: $event })" @upload-failure="ui.showToast('error', '图片上传失败', '正文未发生变化')" />
      <section class="rounded-[var(--radius-card)] border border-border bg-surface p-6"><h2 class="mb-4 text-lg font-semibold">安全预览</h2><MarkdownRenderer :markdown="draftStore.draft.markdown" /></section>
    </template>

    <AppModal :open="createTagModalOpen" title="新增标签" @close="closeCreateTagModal">
      <label class="grid gap-2 text-sm font-medium">标签名称<input v-model="tagName" data-testid="tag-name" class="h-11 rounded-[var(--radius-control)] border border-border bg-surface px-3" :disabled="createTagPending" @keydown.enter="createTag" /></label>
      <template #footer>
        <button type="button" data-testid="cancel-create-tag" class="min-h-11 px-4 disabled:opacity-60" :disabled="createTagPending" @click="closeCreateTagModal">取消</button>
        <button type="button" data-testid="submit-create-tag" class="min-h-11 rounded-[var(--radius-control)] bg-accent px-4 font-semibold text-canvas disabled:opacity-60" :disabled="createTagPending || trimmedTagName === ''" :aria-busy="createTagPending" @click="createTag">{{ createTagPending ? '创建中…' : '创建' }}</button>
      </template>
    </AppModal>

    <AppModal :open="createArticleTypeModalOpen" title="新增分类" @close="closeCreateArticleTypeModal">
      <label class="grid gap-2 text-sm font-medium">分类名称<input v-model="articleTypeName" data-testid="article-type-name" class="h-11 rounded-[var(--radius-control)] border border-border bg-surface px-3" :disabled="createArticleTypePending" @keydown.enter="createArticleType" /></label>
      <template #footer>
        <button type="button" data-testid="cancel-create-article-type" class="min-h-11 px-4 disabled:opacity-60" :disabled="createArticleTypePending" @click="closeCreateArticleTypeModal">取消</button>
        <button type="button" data-testid="submit-create-article-type" class="min-h-11 rounded-[var(--radius-control)] bg-accent px-4 font-semibold text-canvas disabled:opacity-60" :disabled="createArticleTypePending || trimmedArticleTypeName === ''" :aria-busy="createArticleTypePending" @click="createArticleType">{{ createArticleTypePending ? '创建中…' : '创建' }}</button>
      </template>
    </AppModal>
    <AppModal :open="updateTagTarget !== null" title="修改标签" @close="closeUpdateTag">
      <label class="grid gap-2 text-sm font-medium">标签名称<input v-model="updateTagName" data-testid="update-tag-name" maxlength="60" class="h-11 rounded-[var(--radius-control)] border border-border bg-surface px-3" :disabled="updateTagPending" @keydown.enter="updateTag" /></label>
      <p v-if="updateTagName.trim().length > 60" class="mt-2 text-sm text-error" role="alert">标签名称不能超过 60 个字符</p>
      <template #footer>
        <button type="button" data-testid="cancel-update-tag" class="min-h-11 px-4 disabled:opacity-60" :disabled="updateTagPending" @click="closeUpdateTag">取消</button>
        <button type="button" data-testid="submit-update-tag" class="min-h-11 rounded-[var(--radius-control)] bg-accent px-4 font-semibold text-canvas disabled:opacity-60" :disabled="updateTagPending || !updateTagValid" :aria-busy="updateTagPending" @click="updateTag">{{ updateTagPending ? '保存中…' : '保存' }}</button>
      </template>
    </AppModal>

    <AppModal :open="updateArticleTypeTarget !== null" title="修改分类" @close="closeUpdateArticleTypeModal">
      <label class="grid gap-2 text-sm font-medium">分类名称<input v-model="updateArticleTypeName" data-testid="update-article-type-name" maxlength="60" class="h-11 rounded-[var(--radius-control)] border border-border bg-surface px-3" :disabled="updateArticleTypePending" @keydown.enter="updateArticleType" /></label>
      <p v-if="updateArticleTypeName.trim().length > 60" class="mt-2 text-sm text-error" role="alert">分类名称不能超过 60 个字符</p>
      <template #footer>
        <button type="button" data-testid="cancel-update-article-type" class="min-h-11 px-4 disabled:opacity-60" :disabled="updateArticleTypePending" @click="closeUpdateArticleTypeModal">取消</button>
        <button type="button" data-testid="submit-update-article-type" class="min-h-11 rounded-[var(--radius-control)] bg-accent px-4 font-semibold text-canvas disabled:opacity-60" :disabled="updateArticleTypePending || !updateArticleTypeValid" :aria-busy="updateArticleTypePending" @click="updateArticleType">{{ updateArticleTypePending ? '保存中…' : '保存' }}</button>
      </template>
    </AppModal>

    <AppModal :open="deleteArticleTypeTarget !== null" title="确认删除分类" :description="deleteArticleTypeTarget === null ? '' : `将删除“${deleteArticleTypeTarget.name}”，此操作不可撤销。`" @close="closeDeleteArticleTypeModal">
      <template #footer>
        <button type="button" data-testid="cancel-delete-article-type" class="min-h-11 px-4 disabled:opacity-60" :disabled="deleteArticleTypePending" @click="closeDeleteArticleTypeModal">取消</button>
        <button type="button" data-testid="confirm-delete-article-type" class="min-h-11 rounded-[var(--radius-control)] bg-error px-4 font-semibold text-canvas disabled:opacity-60" :disabled="deleteArticleTypePending" :aria-busy="deleteArticleTypePending" @click="deleteArticleType">{{ deleteArticleTypePending ? '删除中…' : '确认删除' }}</button>
      </template>
    </AppModal>
  </div>
</template>
