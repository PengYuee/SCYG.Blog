import type { ArticleType as ArticleTypeResponse, CreateManageArticleTypeData, DeleteManageArticleTypeData, GetManageArticleTypeData, ListManageArticleTypesData, PatchManageArticleTypeData } from "@/request/generated"
import { zArticleType, zArticleTypeList } from "@/request/generated/zod.gen"
import type { HttpTransport } from "@/request/transport"
import type { ArticleTypeDeleteTarget, AuthorArticleType } from "@/services/author-contracts"
import type { ArticleType, ArticleTypeCreate, ArticleTypeUpdateRequest } from "@/types/taxonomy"
import { normalizeImageUrl, parseBoundary } from "@/types/api"

const ARTICLE_TYPE_ROUTE: ListManageArticleTypesData["url"] = "/api/v1/manage/article-types"
const TAXONOMY_PAGE_SIZE = 100

/** 管理端文章分类 API 适配器契约。 */
export interface ArticleTypeApi {
  readonly list: (filter?: string) => Promise<readonly AuthorArticleType[]>
  readonly detail: (id: number) => Promise<AuthorArticleType>
  readonly create: (request: ArticleTypeCreate) => Promise<AuthorArticleType>
  readonly update: (request: ArticleTypeUpdateRequest) => Promise<AuthorArticleType>
  readonly delete: (target: ArticleTypeDeleteTarget) => Promise<void>
}

function mapArticleType(item: ArticleTypeResponse, serverUrl: string): AuthorArticleType {
  return { id: item.id, name: item.name, imageUrl: item.image === null ? null : normalizeImageUrl(item.image, serverUrl), menu: item.menu, version: item.version }
}

function mapArticleTypes(input: unknown, serverUrl: string) {
  const value = parseBoundary(zArticleTypeList, input, "article type dictionary")
  return { items: value.items.map((item) => mapArticleType(item, serverUrl)), totalPages: value.page.totalPages }
}

export function parseArticleTypes(input: unknown, serverUrl: string): readonly ArticleType[] {
  return mapArticleTypes(input, serverUrl).items
}

export function createArticleTypeApi(client: HttpTransport, serverUrl: string): ArticleTypeApi {
  return {
    async list(filter?: string) {
      const query = filter?.trim()
      const params: NonNullable<ListManageArticleTypesData["query"]> = { pageSize: TAXONOMY_PAGE_SIZE, ...(query === undefined || query.length === 0 ? {} : { q: query }) }
      const firstPage = mapArticleTypes((await client.get(ARTICLE_TYPE_ROUTE, { params: { ...params, page: 1 } })).data, serverUrl)
      const items: AuthorArticleType[] = [...firstPage.items]
      for (let page = 2; page <= firstPage.totalPages; page += 1) items.push(...mapArticleTypes((await client.get(ARTICLE_TYPE_ROUTE, { params: { ...params, page } })).data, serverUrl).items)
      return items
    },
    async detail(id: number): Promise<AuthorArticleType> {
      const path: GetManageArticleTypeData["path"] = { articleTypeId: id }
      return mapArticleType(parseBoundary(zArticleType, (await client.get(`${ARTICLE_TYPE_ROUTE}/${path.articleTypeId}`)).data, "article type detail"), serverUrl)
    },
    async create(request: ArticleTypeCreate): Promise<AuthorArticleType> {
      const payload: CreateManageArticleTypeData["body"] = { name: request.name, image: request.image, menu: request.menu }
      return mapArticleType(parseBoundary(zArticleType, (await client.post(ARTICLE_TYPE_ROUTE, payload)).data, "article type create"), serverUrl)
    },
    async update(request: ArticleTypeUpdateRequest): Promise<AuthorArticleType> {
      const changes: PatchManageArticleTypeData["body"] = {}
      if (request.changes.name !== undefined) changes.name = request.changes.name
      if (request.changes.image !== undefined) changes.image = request.changes.image
      if (request.changes.menu !== undefined) changes.menu = request.changes.menu
      const path: PatchManageArticleTypeData["path"] = { articleTypeId: request.id }
      const headers: PatchManageArticleTypeData["headers"] = { "If-Match": `"${request.version}"` }
      return mapArticleType(parseBoundary(zArticleType, (await client.patch(`${ARTICLE_TYPE_ROUTE}/${path.articleTypeId}`, changes, { headers })).data, "article type update"), serverUrl)
    },
    async delete(target: ArticleTypeDeleteTarget): Promise<void> {
      const path: DeleteManageArticleTypeData["path"] = { articleTypeId: target.id }
      const headers: DeleteManageArticleTypeData["headers"] = { "If-Match": `"${target.version}"` }
      await client.delete(`${ARTICLE_TYPE_ROUTE}/${path.articleTypeId}`, { headers })
    },
  }
}
