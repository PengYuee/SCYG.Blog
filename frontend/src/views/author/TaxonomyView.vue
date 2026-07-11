<script setup lang="ts">
import { onMounted, ref } from "vue"
import AppModal from "@/components/shared/AppModal.vue"
import AppToast from "@/components/shared/AppToast.vue"
import { createFakeAuthorRuntime } from "@/services/author-runtime"
import { useUiStore } from "@/stores/ui"
import type { ArticleType, Tag } from "@/types/taxonomy"

/** 显式 Fake 作者运行时。 */ const runtime = createFakeAuthorRuntime()
/** 全局反馈。 */ const ui = useUiStore()
/** 分类列表。 */ const articleTypes = ref<readonly ArticleType[]>([])
/** 标签列表。 */ const tags = ref<readonly Tag[]>([])
/** 创建弹窗类型。 */ const createKind = ref<"articleType" | "tag" | null>(null)
/** 删除目标。 */ const deleteTarget = ref<{ readonly kind: "articleType" | "tag"; readonly id: number; readonly name: string } | null>(null)
/** 新名称。 */ const name = ref("")

/** 刷新 Fake 分类字典。 */
async function refresh(): Promise<void> { articleTypes.value = await runtime.taxonomy.listArticleTypes(); tags.value = await runtime.taxonomy.listTags() }
onMounted(refresh)

/** 通过共享守卫创建分类或标签。 */
async function createItem(): Promise<void> {
  const kind = createKind.value
  if (kind === null || name.value.trim() === "") return
  const result = await runtime.guard.execute("taxonomy", () => kind === "articleType" ? runtime.taxonomy.createArticleType({ name: name.value.trim(), image: null, menu: articleTypes.value.length + 1 }) : runtime.taxonomy.createTag(name.value.trim()))
  if (!result.ok) { ui.showToast("error", "创建被阻止", result.error.reason); return }
  createKind.value = null; name.value = ""; await refresh(); ui.showToast("success", "分类字典已更新")
}

/** 通过共享守卫删除分类或标签。 */
async function deleteItem(): Promise<void> {
  const target = deleteTarget.value
  if (target === null) return
  const result = await runtime.guard.execute("taxonomy", () => target.kind === "articleType" ? runtime.taxonomy.deleteArticleType(target.id) : runtime.taxonomy.deleteTag(target.id))
  if (!result.ok) { ui.showToast("error", "删除被阻止", result.error.reason); return }
  deleteTarget.value = null; await refresh(); ui.showToast("success", "项目已删除")
}
</script>

<template>
  <div class="grid gap-6"><AppToast /><header><p class="text-sm text-text-secondary">受保护的作者工作区</p><h1 class="font-display text-3xl font-bold">分类与标签</h1></header>
    <section class="grid grid-cols-2 gap-6"><div class="rounded-[var(--radius-card)] border border-border bg-surface p-6"><div class="mb-4 flex items-center justify-between"><h2 class="text-lg font-semibold">文章分类</h2><button class="rounded-lg bg-accent px-4 py-3 text-sm font-semibold text-white" @click="createKind = 'articleType'">新建分类</button></div><ul class="divide-y divide-border-subtle"><li v-for="item in articleTypes" :key="item.id" class="flex min-h-14 items-center justify-between"><span>{{ item.name }}</span><button class="min-h-11 px-3 text-error" @click="deleteTarget = { kind: 'articleType', id: item.id, name: item.name }">删除</button></li></ul></div>
      <div class="rounded-[var(--radius-card)] border border-border bg-surface p-6"><div class="mb-4 flex items-center justify-between"><h2 class="text-lg font-semibold">文章标签</h2><button data-testid="create-tag" class="rounded-lg bg-accent px-4 py-3 text-sm font-semibold text-white" @click="createKind = 'tag'">新建标签</button></div><ul class="divide-y divide-border-subtle"><li v-for="item in tags" :key="item.id" class="flex min-h-14 items-center justify-between"><span>{{ item.name }}</span><button class="min-h-11 px-3 text-error" @click="deleteTarget = { kind: 'tag', id: item.id, name: item.name }">删除</button></li></ul></div></section>
    <AppModal :open="createKind !== null" :title="createKind === 'articleType' ? '新建分类' : '新建标签'" @close="createKind = null"><label class="grid gap-2 text-sm font-medium">名称<input v-model="name" class="h-11 rounded-lg border border-border px-3" /></label><template #footer><button class="min-h-11 px-4" @click="createKind = null">取消</button><button class="min-h-11 rounded-lg bg-accent px-4 text-white" @click="createItem">创建</button></template></AppModal>
    <AppModal :open="deleteTarget !== null" title="确认删除" :description="deleteTarget === null ? '' : `将删除“${deleteTarget.name}”，此操作不可撤销。`" @close="deleteTarget = null"><template #footer><button class="min-h-11 px-4" @click="deleteTarget = null">取消</button><button class="min-h-11 rounded-lg bg-error px-4 text-white" @click="deleteItem">确认删除</button></template></AppModal>
  </div>
</template>
