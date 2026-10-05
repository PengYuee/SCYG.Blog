import type { AuthorArticleRepository, AuthorArticleType, AuthorTag, AuthorTaxonomyRepository } from "@/services/author-contracts"
import type { ArticleCreateRequest, ArticleDetail, ArticleUpdateRequest } from "@/types/article"
import type { ArticleTypeCreate, ArticleTypeUpdateRequest, TagUpdateRequest } from "@/types/taxonomy"

/** Fake 作者调用记录，仅供隔离测试使用。 */
export type FakeAuthorCalls = { articleWrites: number; uploads: number; imageDeletes: number; taxonomyWrites: number }

/** 创建仅供测试注入的内存作者仓储。 */
export function createFakeAuthorRepositories(): { readonly articles: AuthorArticleRepository; readonly taxonomy: AuthorTaxonomyRepository; readonly calls: FakeAuthorCalls } {
  const calls: FakeAuthorCalls = { articleWrites: 0, uploads: 0, imageDeletes: 0, taxonomyWrites: 0 }
  const articleTypes: AuthorArticleType[] = [{ id: 1, name: "工程笔记", imageUrl: null, menu: 1, version: 1 }]
  /** Fake 标签状态保留服务端版本以支持并发删除契约。 */
  const tags: AuthorTag[] = [{ id: 1, name: "Vue", version: 1 }, { id: 2, name: "TypeScript", version: 1 }]
  /** 下一个 Fake 标签标识；删除标签不会回退该单调分配器。 */
  let nextTagId = tags.reduce((largestId, tag) => Math.max(largestId, tag.id), 0) + 1
  let nextArticleId = 43
  let detail: ArticleDetail = { id: 42, title: "受保护的富文本写作", slug: "guarded-authoring", digest: "Fake 编辑示例", markdown: "## 编辑模式\n\n这是一篇通过 T3 Markdown 模型载入的文章。", articleTypeId: 1, articleType: { id: 1, name: "工程笔记", imageUrl: null }, tagIds: [1], status: 1, support: 0, comment: 0, visited: 0, version: 1, createdAt: "2026-07-12T00:00:00Z", updatedAt: null }
  const articles: AuthorArticleRepository = {
    async detail() { return detail },
    async create(request: ArticleCreateRequest) {
      calls.articleWrites += 1
      detail = { ...detail, id: nextArticleId, title: request.title, slug: request.slug, digest: request.digest, markdown: request.markdown, articleTypeId: request.articleTypeId, tagIds: [...request.tagIds], status: request.status, version: 1 }
      nextArticleId += 1
      return detail
    },
    async update(request: ArticleUpdateRequest) {
      calls.articleWrites += 1
      detail = { ...detail, id: request.id, ...request.changes, markdown: request.changes.markdown ?? detail.markdown, articleTypeId: request.changes.articleTypeId ?? detail.articleTypeId, tagIds: request.changes.tagIds === undefined ? detail.tagIds : [...request.changes.tagIds], version: request.version + 1, updatedAt: "2026-07-12T00:00:00Z" }
      return detail
    },
    async uploadImage(file) {
      calls.uploads += 1
      return { id: `fake-image-${calls.uploads}`, url: `https://fake.local/images/${encodeURIComponent(file.name)}`, expiresAt: "2026-07-14T00:00:00Z" }
    },
    async deleteImage() { calls.imageDeletes += 1; return true },
  }
  const taxonomy: AuthorTaxonomyRepository = {
    async listArticleTypes() { return articleTypes }, async listTags() { return tags },
    /** 创建并返回可供后续并发删除使用的版本化 Fake 分类。 */
    async createArticleType(request: ArticleTypeCreate) {
      calls.taxonomyWrites += 1
      const created: AuthorArticleType = { id: articleTypes.length + 1, name: request.name, imageUrl: request.image, menu: request.menu, version: 1 }
      articleTypes.push(created)
      return created
    },
    /** 按当前版本更新分类名称并递增 Fake 资源版本。 */
    async updateArticleType(request: ArticleTypeUpdateRequest) {
      calls.taxonomyWrites += 1
      const index = articleTypes.findIndex((item) => item.id === request.id)
      if (index < 0) throw new Error("分类不存在")
      const current = articleTypes[index]
      if (current === undefined) throw new Error("分类不存在")
      const updated: AuthorArticleType = { ...current, ...(request.changes.name === undefined ? {} : { name: request.changes.name }), version: request.version + 1 }
      articleTypes[index] = updated
      return updated
    },
    /** 按窄删除目标移除 Fake 分类。 */
    async deleteArticleType(target) { calls.taxonomyWrites += 1; const index = articleTypes.findIndex((item) => item.id === target.id); if (index >= 0) articleTypes.splice(index, 1) },
    /** 创建并返回可供后续并发删除使用的完整版本化 Fake 标签。 */
    async createTag(name) {
      calls.taxonomyWrites += 1
      const created: AuthorTag = { id: nextTagId, name, version: 1 }
      nextTagId += 1
      tags.push(created)
      return created
    },
    /** 按当前版本更新标签名称并递增 Fake 资源版本。 */
    async updateTag(request: TagUpdateRequest) {
      calls.taxonomyWrites += 1
      const index = tags.findIndex((item) => item.id === request.id)
      if (index < 0) throw new Error("标签不存在")
      const current = tags[index]
      if (current === undefined) throw new Error("标签不存在")
      const updated: AuthorTag = { ...current, ...(request.changes.name === undefined ? {} : { name: request.changes.name }), version: request.version + 1 }
      tags[index] = updated
      return updated
    },
    /** 按版本化删除目标中的标签标识移除 Fake 标签。 */
    async deleteTag(target) { calls.taxonomyWrites += 1; const index = tags.findIndex((item) => item.id === target.id); if (index >= 0) tags.splice(index, 1) },
  }
  return { articles, taxonomy, calls }
}
