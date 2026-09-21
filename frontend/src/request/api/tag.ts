import { z } from "zod"
import type { HttpTransport } from "@/request/transport"
import type { AuthorTag, TagDeleteTarget } from "@/services/author-contracts"
import type { Tag, TagUpdateRequest } from "@/types/taxonomy"
import { parseBoundary } from "@/types/api"
import { pageSchema, tagSchema } from "./schemas"

const dictionarySchema = z.strictObject({ items: z.array(tagSchema), page: pageSchema })
const TAG_ROUTE = "/api/v1/tags"
const TAXONOMY_PAGE_SIZE = 100

/** 将服务端标签资源映射为作者侧版本化标签。 */
function mapTag(item: z.infer<typeof tagSchema>): AuthorTag {
  return { id: item.id, name: item.name, version: item.version }
}

/** 将标签分页响应映射为作者侧版本化资源。 */
function mapTags(input: unknown) {
  const value = parseBoundary(dictionarySchema, input, "标签字典")
  return { items: value.items.map(mapTag), totalPages: value.page.total_pages }
}

/** 解析当前标签字典包络，保持公共只读标签消费兼容。 */
export function parseTags(input: unknown): readonly Tag[] {
  return mapTags(input).items.map((item) => ({ id: item.id, name: item.name }))
}

/** 标签 API 的类型化适配器。 */
export function createTagApi(client: HttpTransport) {
  return {
    /** 顺序获取全部带版本标签分页。 */
    async list(filter?: string): Promise<readonly AuthorTag[]> {
      const query = filter?.trim()
      const params = { page_size: TAXONOMY_PAGE_SIZE, ...(query === undefined || query.length === 0 ? {} : { q: query }) }
      const firstResponse = await client.get(TAG_ROUTE, { params: { ...params, page: 1 } })
      const firstPage = mapTags(firstResponse.data)
      const items: AuthorTag[] = [...firstPage.items]
      for (let page = 2; page <= firstPage.totalPages; page += 1) {
        const response = await client.get(TAG_ROUTE, { params: { ...params, page } })
        items.push(...mapTags(response.data).items)
      }
      return items
    },
    /** 获取单个标签并解析版本化资源。 */
    async detail(id: number): Promise<AuthorTag> {
      const response = await client.get(`${TAG_ROUTE}/${id}`)
      return mapTag(parseBoundary(tagSchema, response.data, "tag detail"))
    },
    /** 创建标签并解析未包裹的版本化资源。 */
    async create(name: string): Promise<AuthorTag> {
      const response = await client.post(TAG_ROUTE, { name })
      return mapTag(parseBoundary(tagSchema, response.data, "创建标签响应"))
    },
    /** 使用强实体标签局部更新标签。 */
    async update(request: TagUpdateRequest): Promise<AuthorTag> {
      const changes = request.changes.name === undefined ? {} : { name: request.changes.name }
      const response = await client.patch(`${TAG_ROUTE}/${request.id}`, changes, { headers: { "If-Match": `"${request.version}"` } })
      return mapTag(parseBoundary(tagSchema, response.data, "tag update"))
    },
    /** 使用强实体标签删除当前版本标签。 */
    async delete(target: TagDeleteTarget): Promise<void> {
      await client.delete(`${TAG_ROUTE}/${target.id}`, { headers: { "If-Match": `"${target.version}"` } })
    },
  }
}
