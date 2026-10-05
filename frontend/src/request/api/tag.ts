import type { CreateManageTagData, DeleteManageTagData, GetManageTagData, ListManageTagsData, PatchManageTagData, Tag as TagResponse } from "@/request/generated"
import { zTag, zTagList } from "@/request/generated/zod.gen"
import type { HttpTransport } from "@/request/transport"
import type { AuthorTag, TagDeleteTarget } from "@/services/author-contracts"
import type { Tag, TagUpdateRequest } from "@/types/taxonomy"
import { parseBoundary } from "@/types/api"

const TAG_ROUTE: ListManageTagsData["url"] = "/api/v1/manage/tags"
const TAXONOMY_PAGE_SIZE = 100

/** 管理端标签 API 适配器契约。 */
export interface TagApi {
  readonly list: (filter?: string) => Promise<readonly AuthorTag[]>
  readonly detail: (id: number) => Promise<AuthorTag>
  readonly create: (name: string) => Promise<AuthorTag>
  readonly update: (request: TagUpdateRequest) => Promise<AuthorTag>
  readonly delete: (target: TagDeleteTarget) => Promise<void>
}

function mapTag(item: TagResponse): AuthorTag {
  return { id: item.id, name: item.name, version: item.version }
}

function mapTags(input: unknown) {
  const value = parseBoundary(zTagList, input, "标签字典")
  return { items: value.items.map(mapTag), totalPages: value.page.totalPages }
}

export function parseTags(input: unknown): readonly Tag[] {
  return mapTags(input).items.map((item) => ({ id: item.id, name: item.name }))
}
export function createTagApi(client: HttpTransport): TagApi {
  return {
    async list(filter?: string): Promise<readonly AuthorTag[]> {
      const query = filter?.trim()
      const params: NonNullable<ListManageTagsData["query"]> = { pageSize: TAXONOMY_PAGE_SIZE, ...(query === undefined || query.length === 0 ? {} : { q: query }) }
      const firstPage = mapTags((await client.get(TAG_ROUTE, { params: { ...params, page: 1 } })).data)
      const items: AuthorTag[] = [...firstPage.items]
      for (let page = 2; page <= firstPage.totalPages; page += 1) items.push(...mapTags((await client.get(TAG_ROUTE, { params: { ...params, page } })).data).items)
      return items
    },
    async detail(id: number): Promise<AuthorTag> {
      const path: GetManageTagData["path"] = { tagId: id }
      return mapTag(parseBoundary(zTag, (await client.get(`${TAG_ROUTE}/${path.tagId}`)).data, "tag detail"))
    },
    async create(name: string): Promise<AuthorTag> {
      const payload: CreateManageTagData["body"] = { name }
      return mapTag(parseBoundary(zTag, (await client.post(TAG_ROUTE, payload)).data, "创建标签响应"))
    },
    async update(request: TagUpdateRequest): Promise<AuthorTag> {
      const changes: PatchManageTagData["body"] = request.changes.name === undefined ? {} : { name: request.changes.name }
      const path: PatchManageTagData["path"] = { tagId: request.id }
      const headers: PatchManageTagData["headers"] = { "If-Match": `"${request.version}"` }
      return mapTag(parseBoundary(zTag, (await client.patch(`${TAG_ROUTE}/${path.tagId}`, changes, { headers })).data, "tag update"))
    },
    async delete(target: TagDeleteTarget): Promise<void> {
      const path: DeleteManageTagData["path"] = { tagId: target.id }
      const headers: DeleteManageTagData["headers"] = { "If-Match": `"${target.version}"` }
      await client.delete(`${TAG_ROUTE}/${path.tagId}`, { headers })
    },
  }
}
