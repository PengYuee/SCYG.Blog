<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import AppModal from "@/components/shared/AppModal.vue"
import AppToast from "@/components/shared/AppToast.vue"
import { useApiServices } from "@/request/api-services"
import { HttpRequestError } from "@/request/http-error"
import { createAuthorRuntime, type AuthorRuntime } from "@/services/author-runtime"
import { useAuthSession } from "@/services/auth-session"
import { TAXONOMY_MUTATION_DOMAIN } from "@/services/mutation-guard"
import { useUiStore } from "@/stores/ui"
import type { ArticleTypeDeleteTarget, AuthorArticleType, AuthorTag, TagDeleteTarget } from "@/services/author-contracts"


/** 测试可注入作者运行时；路由页面使用真实 API 与认证状态。 */
const props = defineProps<{ readonly runtime?: AuthorRuntime }>()
/** 当前作者运行时。 */
const runtime = props.runtime ?? createAuthorRuntime(useApiServices(), useAuthSession().currentState)
/** 全局反馈。 */ const ui = useUiStore()
/** 分类列表。 */ const articleTypes = ref<readonly AuthorArticleType[]>([])
/** 带并发删除版本的标签列表。 */ const tags = ref<readonly AuthorTag[]>([])
/** 创建弹窗类型。 */ const createKind = ref<"articleType" | "tag" | null>(null)
/** 分类删除确认框目标。 */
type ArticleTypeDeleteConfirmation = ArticleTypeDeleteTarget & {
  /** 目标字典类型。 */ readonly kind: "articleType"
  /** 确认框展示名称。 */ readonly name: string
}
/** 标签删除确认框目标。 */
type TagDeleteConfirmation = TagDeleteTarget & {
  /** 目标字典类型。 */ readonly kind: "tag"
  /** 确认框展示名称。 */ readonly name: string
}
/** 分类或标签的可确认删除目标。 */
type DeleteTarget = ArticleTypeDeleteConfirmation | TagDeleteConfirmation
/** 删除目标。 */ const deleteTarget = ref<DeleteTarget | null>(null)
/** 待编辑的分类。 */ const updateTarget = ref<AuthorArticleType | null>(null)
/** 分类修改输入。 */ const updateName = ref("")
/** 分类修改请求状态。 */ const updatePending = ref(false)
/** 分类修改提交值。 */ const trimmedUpdateName = computed(() => updateName.value.trim())
/** 分类修改输入是否有效。 */ const updateValid = computed(() => trimmedUpdateName.value.length > 0 && trimmedUpdateName.value.length <= 60 && updateTarget.value?.name !== trimmedUpdateName.value)
/** 待编辑的标签。 */ const tagUpdateTarget = ref<AuthorTag | null>(null)
/** 标签修改输入。 */ const tagUpdateName = ref("")
/** 标签修改请求状态。 */ const tagUpdatePending = ref(false)
/** 标签修改提交值。 */ const trimmedTagUpdateName = computed(() => tagUpdateName.value.trim())
/** 标签修改输入是否有效。 */ const tagUpdateValid = computed(() => trimmedTagUpdateName.value.length > 0 && trimmedTagUpdateName.value.length <= 60 && tagUpdateTarget.value?.name !== trimmedTagUpdateName.value)
/** 分类或标签任一修改请求期间锁定字典操作。 */ const taxonomyUpdatePending = computed(() => updatePending.value || tagUpdatePending.value)
/** 新名称。 */ const name = ref("")
/** 初始化状态，失败时呈现重试操作。 */ const initialization = ref<"loading" | "ready" | "error">("loading")

/** 刷新当前作者运行时的分类字典。 */
async function refresh(): Promise<void> {
  initialization.value = "loading"
  const articleTypesRequest = Promise.resolve().then(() => runtime.taxonomy.listArticleTypes())
  const tagsRequest = Promise.resolve().then(() => runtime.taxonomy.listTags())
  await Promise.all([articleTypesRequest, tagsRequest]).then(
    ([nextArticleTypes, nextTags]) => { articleTypes.value = nextArticleTypes; tags.value = nextTags; initialization.value = "ready" },
    () => { initialization.value = "error"; ui.showToast("error", "分类加载失败", "请重试载入分类与标签") },
  )
}
onMounted(refresh)

/** 通过共享守卫创建分类或标签。 */
async function createItem(): Promise<void> {
  const kind = createKind.value
  if (kind === null || name.value.trim() === "") return
  const result = await runtime.guard.execute(TAXONOMY_MUTATION_DOMAIN, () => kind === "articleType" ? runtime.taxonomy.createArticleType({ name: name.value.trim(), image: null, menu: articleTypes.value.length + 1 }).then(() => undefined) : runtime.taxonomy.createTag(name.value.trim()).then(() => undefined))
  if (!result.ok) { ui.showToast("error", "创建被阻止", result.error.reason); return }
  createKind.value = null; name.value = ""; await refresh(); ui.showToast("success", "分类字典已更新")
}

/** 通过共享守卫删除分类或标签。 */
async function deleteItem(): Promise<void> {
  const target = deleteTarget.value
  if (target === null) return
  const result = await runtime.guard.execute(TAXONOMY_MUTATION_DOMAIN, () => target.kind === "articleType" ? runtime.taxonomy.deleteArticleType({ id: target.id, version: target.version }) : runtime.taxonomy.deleteTag({ id: target.id, version: target.version }))
  if (!result.ok) { ui.showToast("error", "删除被阻止", result.error.reason); return }
  deleteTarget.value = null; await refresh(); ui.showToast("success", "项目已删除")
}
/** 打开分类修改弹框。 */
function requestUpdateArticleType(item: AuthorArticleType): void {
  if (updatePending.value) return
  updateTarget.value = item
  updateName.value = item.name
}

/** 关闭未提交的分类修改弹框。 */
function closeUpdateArticleType(): void {
  if (updatePending.value) return
  updateTarget.value = null
  updateName.value = ""
}

/** 修改分类名称并收敛服务端返回的完整资源。 */
async function updateArticleType(): Promise<void> {
  const target = updateTarget.value
  if (target === null || updatePending.value || !updateValid.value) return
  updatePending.value = true
  const result = await Promise.resolve().then(() => runtime.guard.execute(TAXONOMY_MUTATION_DOMAIN, () => runtime.taxonomy.updateArticleType({ id: target.id, version: target.version, changes: { name: trimmedUpdateName.value } }))).then(
    (value) => value,
    async (error: unknown) => {
      if (error instanceof HttpRequestError && error.status === 409) {
        updatePending.value = false
        ui.showToast("error", "分类修改失败", "分类名称已存在")
        return null
      }
      if (error instanceof HttpRequestError && (error.status === 404 || error.status === 412)) {
        await refresh().catch(() => undefined)
        const refreshed = articleTypes.value.find((item) => item.id === target.id)
        if (refreshed === undefined) {
          updateTarget.value = null
          updateName.value = ""
          updatePending.value = false
          ui.showToast("info", "分类状态已更新", "该分类已不存在")
          return null
        }
        updateTarget.value = refreshed
        updateName.value = refreshed.name
        updatePending.value = false
        ui.showToast("warning", "分类版本已更新", "分类已被其他操作修改，请确认最新名称后再保存")
        return null
      }
      updatePending.value = false
      ui.showToast("error", "修改分类失败", error instanceof HttpRequestError ? "分类服务请求失败，请重试" : "操作未完成，请重试")
      return null
    },
  )
  if (result === null) return
  if (!result.ok) {
    updatePending.value = false
    ui.showToast("error", "修改分类被阻止", result.error.reason)
    return
  }
  const updated = result.value
  articleTypes.value = [...articleTypes.value.filter((item) => item.id !== updated.id), updated]
  updateTarget.value = null
  updateName.value = ""
  updatePending.value = false
  ui.showToast("success", "分类已修改")
}
/** 打开标签修改弹框。 */
function requestUpdateTag(item: AuthorTag): void {
  if (taxonomyUpdatePending.value) return
  tagUpdateTarget.value = item
  tagUpdateName.value = item.name
}


/** 关闭未提交的标签修改弹框。 */
function closeUpdateTag(): void {
  if (tagUpdatePending.value) return
  tagUpdateTarget.value = null
  tagUpdateName.value = ""
}

/** 修改标签名称并处理版本冲突。 */
async function updateTag(): Promise<void> {
  const target = tagUpdateTarget.value
  if (target === null || tagUpdatePending.value || !tagUpdateValid.value) return
  tagUpdatePending.value = true
  const result = await Promise.resolve().then(() => runtime.guard.execute(TAXONOMY_MUTATION_DOMAIN, () => runtime.taxonomy.updateTag({ id: target.id, version: target.version, changes: { name: trimmedTagUpdateName.value } }))).then(
    (value) => value,
    async (error: unknown) => {
      if (error instanceof HttpRequestError && error.status === 409) {
        tagUpdatePending.value = false
        ui.showToast("error", "修改标签失败", "标签名称已存在")
        return null
      }
      if (error instanceof HttpRequestError && (error.status === 404 || error.status === 412)) {
        await refresh().catch(() => undefined)
        const refreshed = tags.value.find((item) => item.id === target.id)
        if (refreshed === undefined) {
          tagUpdateTarget.value = null
          tagUpdateName.value = ""
          tagUpdatePending.value = false
          ui.showToast("info", "标签状态已更新", "该标签已不存在")
          return null
        }
        tagUpdateTarget.value = refreshed
        tagUpdateName.value = refreshed.name
        tagUpdatePending.value = false
        ui.showToast("warning", "标签版本已更新", "标签已被其他操作修改，请确认最新名称后再保存")
        return null
      }
      tagUpdatePending.value = false
      ui.showToast("error", "修改标签失败", error instanceof HttpRequestError ? "标签服务请求失败，请重试" : "操作未完成，请重试")
      return null
    },
  )
  if (result === null) return
  if (!result.ok) {
    tagUpdatePending.value = false
    ui.showToast("error", "修改标签被阻止", result.error.reason)
    return
  }
  const updated = result.value
  tags.value = [...tags.value.filter((item) => item.id !== updated.id), updated]
  tagUpdateTarget.value = null
  tagUpdateName.value = ""
  tagUpdatePending.value = false
  ui.showToast("success", "标签已修改")
}
</script>

<template>
  <div class="grid gap-6"><AppToast /><header><p class="text-sm text-text-secondary">受保护的作者工作区</p><h1 class="font-display text-3xl font-bold">分类与标签</h1></header>
    <p v-if="initialization === 'loading'" role="status" class="rounded-[var(--radius-card)] border border-border bg-surface p-6">正在载入分类与标签…</p>
    <section v-else-if="initialization === 'error'" role="alert" class="rounded-[var(--radius-card)] border border-error bg-error-soft p-6"><h2 class="font-semibold text-error">分类加载失败</h2><button data-testid="retry-taxonomy-init" class="mt-4 rounded-lg bg-accent px-4 py-3 font-semibold text-canvas" @click="refresh">重新载入</button></section>
    <section v-else class="grid grid-cols-2 gap-6"><div class="rounded-[var(--radius-card)] border border-border bg-surface p-6"><div class="mb-4 flex items-center justify-between"><h2 class="text-lg font-semibold">文章分类</h2><button class="rounded-lg bg-accent px-4 py-3 text-sm font-semibold text-canvas disabled:opacity-60" :disabled="updatePending" @click="createKind = 'articleType'">新建分类</button></div><ul class="divide-y divide-border-subtle"><li v-for="item in articleTypes" :key="item.id" class="flex min-h-14 items-center justify-between"><span>{{ item.name }}</span><span class="flex items-center"><button type="button" class="min-h-11 px-3 text-accent disabled:opacity-60" :disabled="updatePending" :aria-label="`修改分类“${item.name}”`" @click="requestUpdateArticleType(item)">修改</button><button type="button" class="min-h-11 px-3 text-error disabled:opacity-60" :disabled="updatePending" :aria-label="`删除分类“${item.name}”`" @click="deleteTarget = { kind: 'articleType', id: item.id, version: item.version, name: item.name }">删除</button></span></li></ul></div>
      <div class="rounded-[var(--radius-card)] border border-border bg-surface p-6"><div class="mb-4 flex items-center justify-between"><h2 class="text-lg font-semibold">文章标签</h2><button data-testid="create-tag" class="rounded-lg bg-accent px-4 py-3 text-sm font-semibold text-canvas disabled:opacity-60" :disabled="taxonomyUpdatePending" @click="createKind = 'tag'">新建标签</button></div><ul class="divide-y divide-border-subtle"><li v-for="item in tags" :key="item.id" class="flex min-h-14 items-center justify-between"><span>{{ item.name }}</span><span class="flex items-center"><button type="button" class="min-h-11 px-3 text-accent disabled:opacity-60" :disabled="taxonomyUpdatePending" :aria-label="`修改标签“${item.name}”`" @click="requestUpdateTag(item)">修改</button><button type="button" class="min-h-11 px-3 text-error disabled:opacity-60" :disabled="taxonomyUpdatePending" :aria-label="`删除标签“${item.name}”`" @click="deleteTarget = { kind: 'tag', id: item.id, version: item.version, name: item.name }">删除</button></span></li></ul></div></section>
    <AppModal :open="createKind !== null" :title="createKind === 'articleType' ? '新建分类' : '新建标签'" @close="createKind = null"><label class="grid gap-2 text-sm font-medium">名称<input v-model="name" class="h-11 rounded-lg border border-border px-3" /></label><template #footer><button class="min-h-11 px-4" @click="createKind = null">取消</button><button class="min-h-11 rounded-lg bg-accent px-4 text-canvas" @click="createItem">创建</button></template></AppModal>
    <AppModal :open="tagUpdateTarget !== null" title="修改标签" @close="closeUpdateTag"><label class="grid gap-2 text-sm font-medium">标签名称<input v-model="tagUpdateName" data-testid="taxonomy-update-tag-name" maxlength="60" class="h-11 rounded-lg border border-border px-3" :disabled="tagUpdatePending" /></label><p v-if="tagUpdateName.trim().length > 60" role="alert" class="mt-2 text-sm text-error">标签名称不能超过 60 个字符</p><template #footer><button type="button" data-testid="cancel-taxonomy-update-tag" class="min-h-11 px-4 disabled:opacity-60" :disabled="tagUpdatePending" @click="closeUpdateTag">取消</button><button type="button" data-testid="submit-taxonomy-update-tag" class="min-h-11 rounded-lg bg-accent px-4 text-canvas disabled:opacity-60" :disabled="tagUpdatePending || !tagUpdateValid" :aria-busy="tagUpdatePending" @click="updateTag">{{ tagUpdatePending ? '保存中…' : '保存' }}</button></template></AppModal>
    <AppModal :open="updateTarget !== null" title="修改分类" @close="closeUpdateArticleType"><label class="grid gap-2 text-sm font-medium">分类名称<input v-model="updateName" data-testid="taxonomy-update-article-type-name" maxlength="60" class="h-11 rounded-lg border border-border px-3" :disabled="updatePending" /></label><p v-if="updateName.trim().length > 60" role="alert" class="mt-2 text-sm text-error">分类名称不能超过 60 个字符</p><template #footer><button type="button" data-testid="cancel-taxonomy-update" class="min-h-11 px-4 disabled:opacity-60" :disabled="updatePending" @click="closeUpdateArticleType">取消</button><button type="button" data-testid="submit-taxonomy-update" class="min-h-11 rounded-lg bg-accent px-4 text-canvas disabled:opacity-60" :disabled="updatePending || !updateValid" :aria-busy="updatePending" @click="updateArticleType">{{ updatePending ? '保存中…' : '保存' }}</button></template></AppModal>
    <AppModal :open="deleteTarget !== null" title="确认删除" :description="deleteTarget === null ? '' : `将删除“${deleteTarget.name}”，此操作不可撤销。`" @close="deleteTarget = null"><template #footer><button class="min-h-11 px-4" @click="deleteTarget = null">取消</button><button class="min-h-11 rounded-lg bg-error px-4 text-canvas" @click="deleteItem">确认删除</button></template></AppModal>
  </div>
</template>
