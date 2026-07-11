import type { AuthorArticleRepository, AuthorTaxonomyRepository } from "@/services/author-contracts"
import type { ArticleDetail, ArticleUpdate, ArticleWrite } from "@/types/article"
import type { ArticleType, ArticleTypeCreate, Tag } from "@/types/taxonomy"

/** Fake 作者调用记录，供测试与展示证据使用。 */
export type FakeAuthorCalls = { articleWrites: number; uploads: number; imageDeletes: number; taxonomyWrites: number }

/** 创建仅由显式开发开关使用的内存作者仓储。 */
export function createFakeAuthorRepositories(): { readonly articles: AuthorArticleRepository; readonly taxonomy: AuthorTaxonomyRepository; readonly calls: FakeAuthorCalls } {
  const calls: FakeAuthorCalls = { articleWrites: 0, uploads: 0, imageDeletes: 0, taxonomyWrites: 0 }
  const articleTypes: ArticleType[] = [{ id: 1, name: "工程笔记", imageUrl: null, menu: 1 }]
  const tags: Tag[] = [{ id: 1, name: "Vue" }, { id: 2, name: "TypeScript" }]
  const detail: ArticleDetail = { id: 42, title: "受保护的富文本写作", slug: "guarded-authoring", digest: "Fake 编辑示例", markdown: "## 编辑模式\n\n这是一篇通过 T3 Markdown 模型载入的文章。", articleTypeId: 1, tagIds: [1], status: 1, support: 0, comment: 0, visited: 0, version: 1, createdAt: "2026-07-12T00:00:00Z", updatedAt: null }
  const articles: AuthorArticleRepository = {
    async detail() { return detail },
    async create(_request: ArticleWrite) { calls.articleWrites += 1; return true },
    async update(_request: ArticleUpdate) { calls.articleWrites += 1; return true },
    async uploadImage(file) { calls.uploads += 1; return `https://fake.local/images/${encodeURIComponent(file.name)}` },
    async deleteImage() { calls.imageDeletes += 1; return true },
  }
  const taxonomy: AuthorTaxonomyRepository = {
    async listArticleTypes() { return articleTypes }, async listTags() { return tags },
    async createArticleType(request: ArticleTypeCreate) { calls.taxonomyWrites += 1; articleTypes.push({ id: articleTypes.length + 1, name: request.name, imageUrl: null, menu: request.menu }); return true },
    async deleteArticleType(id) { calls.taxonomyWrites += 1; const index = articleTypes.findIndex((item) => item.id === id); if (index >= 0) articleTypes.splice(index, 1); return true },
    async createTag(name) { calls.taxonomyWrites += 1; tags.push({ id: tags.length + 1, name }); return true },
    async deleteTag(id) { calls.taxonomyWrites += 1; const index = tags.findIndex((item) => item.id === id); if (index >= 0) tags.splice(index, 1); return true },
  }
  return { articles, taxonomy, calls }
}
