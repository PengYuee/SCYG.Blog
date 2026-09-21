import { z } from "zod"
import type { HttpTransport } from "@/request/transport"
import type { ArticleTypeDeleteTarget, AuthorArticleType } from "@/services/author-contracts"
import type { ArticleType, ArticleTypeCreate, ArticleTypeUpdateRequest } from "@/types/taxonomy"
import { normalizeImageUrl, parseBoundary } from "@/types/api"
import { articleTypeSchema, pageSchema } from "./schemas"

const dictionarySchema = z.strictObject({ items: z.array(articleTypeSchema), page: pageSchema })
const ARTICLE_TYPE_ROUTE = "/api/v1/article-types"
const TAXONOMY_PAGE_SIZE = 100

/** 将服务端分类资源映射为作者侧版本化分类。 */
function mapArticleType(item: z.infer<typeof articleTypeSchema>, serverUrl: string): AuthorArticleType {
  return { id: item.id, name: item.name, imageUrl: item.image === null ? null : normalizeImageUrl(item.image, serverUrl), menu: item.meun, version: item.version }
}

/** 将分类响应项映射为领域分类。 */
function mapArticleTypes(input: unknown, serverUrl: string) {
  const value = parseBoundary(dictionarySchema, input, "article type dictionary")
  return { items: value.items.map((item) => mapArticleType(item, serverUrl)), totalPages: value.page.total_pages }
}

/** 解析当前分类字典包络。 */
export function parseArticleTypes(input: unknown, serverUrl: string): readonly ArticleType[] {
  return mapArticleTypes(input, serverUrl).items
}

/** 分类 API 的类型化适配器。 */
export function createArticleTypeApi(client: HttpTransport, serverUrl: string) {
  return {
    /** 顺序获取全部带版本分类分页。 */
    async list(filter?: string) {
      const query = filter?.trim()
      const params = { page_size: TAXONOMY_PAGE_SIZE, ...(query === undefined || query.length === 0 ? {} : { q: query }) }
      const firstResponse = await client.get(ARTICLE_TYPE_ROUTE, { params: { ...params, page: 1 } })
      const firstPage = mapArticleTypes(firstResponse.data, serverUrl)
      const items: AuthorArticleType[] = [...firstPage.items]
      for (let page = 2; page <= firstPage.totalPages; page += 1) {
        const response = await client.get(ARTICLE_TYPE_ROUTE, { params: { ...params, page } })
        items.push(...mapArticleTypes(response.data, serverUrl).items)
      }
      return items
    },
    /** 获取单个分类并解析版本化资源。 */
    async detail(id: number): Promise<AuthorArticleType> {
      const response = await client.get(`${ARTICLE_TYPE_ROUTE}/${id}`)
      return mapArticleType(parseBoundary(articleTypeSchema, response.data, "article type detail"), serverUrl)
    },
    /** 创建分类并解析未包裹的版本化资源。 */
    async create(request: ArticleTypeCreate): Promise<AuthorArticleType> {
      const response = await client.post(ARTICLE_TYPE_ROUTE, { name: request.name, image: request.image, meun: request.menu })
      return mapArticleType(parseBoundary(articleTypeSchema, response.data, "article type create"), serverUrl)
    },
    /** 使用强实体标签局部更新分类。 */
    async update(request: ArticleTypeUpdateRequest): Promise<AuthorArticleType> {
      const changes: Record<string, unknown> = {}
      if (request.changes.name !== undefined) changes["name"] = request.changes.name
      if (request.changes.image !== undefined) changes["image"] = request.changes.image
      if (request.changes.menu !== undefined) changes["meun"] = request.changes.menu
      const response = await client.patch(`${ARTICLE_TYPE_ROUTE}/${request.id}`, changes, { headers: { "If-Match": `"${request.version}"` } })
      return mapArticleType(parseBoundary(articleTypeSchema, response.data, "article type update"), serverUrl)
    },
    /** 使用强实体标签删除当前版本分类。 */
    async delete(target: ArticleTypeDeleteTarget): Promise<void> {
      await client.delete(`${ARTICLE_TYPE_ROUTE}/${target.id}`, { headers: { "If-Match": `"${target.version}"` } })
    },
  }
}
