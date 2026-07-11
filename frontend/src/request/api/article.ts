import { z } from "zod"
import type { HttpTransport } from "@/request/transport"
import type { ArticleDetail, ArticleListRequest, ArticleUpdate, ArticleWrite } from "@/types/article"
import type { PageResult } from "@/types/api"
import { normalizeImageUrl, parseBoundary } from "@/types/api"
import { articleSchema, mutationSchema, pageSchema } from "./schemas"

const listSchema = z.strictObject({ items: z.array(articleSchema), page: pageSchema })
const uploadSchema = z.union([z.string(), z.strictObject({ url: z.string() }), z.strictObject({ data: z.string() })])

/** 将当前 API 文章映射为领域 Markdown 模型。 */
export function parseArticleDetail(input: unknown): ArticleDetail {
  const value = parseBoundary(articleSchema, input, "article detail")
  return { id: value.id, title: value.title, slug: value.slug, digest: value.digest, markdown: value.content, articleTypeId: value.article_type_id, tagIds: value.tag_ids, status: value.status, support: value.support, comment: value.comment, visited: value.visited, version: value.version, createdAt: value.created_at, updatedAt: value.updated_at }
}

/** 解析当前 API 文章分页包络。 */
export function parseArticleList(input: unknown): PageResult<ArticleDetail> {
  const value = parseBoundary(listSchema, input, "article list")
  return { items: value.items.map(parseArticleDetail), pageIndex: value.page.number, pageSize: value.page.size, totalItems: value.page.total_items, totalPages: value.page.total_pages }
}

/** 旧文章 API 的类型化适配器。 */
export function createArticleApi(client: HttpTransport, serverUrl: string) {
  const body = (request: ArticleWrite) => ({ title: request.title, body: request.markdown, digest: request.digest, tagIds: request.tagIds, articleTypeId: request.articleTypeId })
  return {
    /** 获取文章列表。 */ async list(request: ArticleListRequest) { const response = await client.get("/Article/GetArticleList", { params: request }); return parseArticleList(response.data) },
    /** 获取文章详情。 */ async detail(id: number) { const response = await client.get("/Article/GetArticle", { params: { id } }); return parseArticleDetail(response.data) },
    /** 创建文章。 */ async create(request: ArticleWrite) { const response = await client.post("/Article/CreateArticle", body(request)); return parseBoundary(mutationSchema, response.data, "article create") },
    /** 更新文章。 */ async update(request: ArticleUpdate) { const response = await client.put("/Article/UpdateArticle", { id: request.id, ...body(request) }); return parseBoundary(mutationSchema, response.data, "article update") },
    /** 上传文章图片，浏览器自动设置 multipart boundary。 */ async uploadImage(image: File) { const form = new FormData(); form.append("image", image); const response = await client.post("/Article/UpLoadArticleImage", form); const value = parseBoundary(uploadSchema, response.data, "image upload"); const location = typeof value === "string" ? value : "url" in value ? value.url : value.data; return normalizeImageUrl(location, serverUrl) },
    /** 删除文章图片。 */ async deleteImage(imageName: string) { const response = await client.delete("/Article/DeleteArticleImage", { params: { imageName } }); return parseBoundary(mutationSchema, response.data, "image delete") },
  }
}
