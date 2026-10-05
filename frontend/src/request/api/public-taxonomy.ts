import type { ListPublicArticleTypesData, ListPublicTagsData, PublicArticleType, PublicTag } from "@/request/generated"
import { zPublicArticleTypeList, zPublicTagList } from "@/request/generated/zod.gen"
import type { HttpTransport } from "@/request/transport"
import type { ArticleType, Tag } from "@/types/taxonomy"
import { normalizeImageUrl, parseBoundary } from "@/types/api"

const PUBLIC_PAGE_SIZE = 100
const PUBLIC_ARTICLE_TYPE_ROUTE: ListPublicArticleTypesData["url"] = "/api/v1/article-types"
const PUBLIC_TAG_ROUTE: ListPublicTagsData["url"] = "/api/v1/tags"

export type PublicArticleTypeView = ArticleType & Pick<PublicArticleType, "articleCount">
export type PublicTagView = Tag & Pick<PublicTag, "articleCount">

/** 公开分类和标签 API 适配器契约。 */
export interface PublicTaxonomyApi {
  readonly listArticleTypes: (filter?: string) => Promise<readonly PublicArticleTypeView[]>
  readonly listTags: (filter?: string) => Promise<readonly PublicTagView[]>
}

function mapArticleType(item: PublicArticleType, serverUrl: string): PublicArticleTypeView {
  return { id: item.id, name: item.name, imageUrl: item.image === null ? null : normalizeImageUrl(item.image, serverUrl), menu: 0, articleCount: item.articleCount }
}

function mapTag(item: PublicTag): PublicTagView {
  return { id: item.id, name: item.name, articleCount: item.articleCount }
}

function articleTypePage(input: unknown, serverUrl: string) {
  const value = parseBoundary(zPublicArticleTypeList, input, "public article type list")
  return { items: value.items.map((item) => mapArticleType(item, serverUrl)), totalPages: value.page.totalPages }
}

function tagPage(input: unknown) {
  const value = parseBoundary(zPublicTagList, input, "public tag list")
  return { items: value.items.map(mapTag), totalPages: value.page.totalPages }
}

export function createPublicTaxonomyApi(client: HttpTransport, serverUrl: string): PublicTaxonomyApi {
  return {
    async listArticleTypes(filter?: string): Promise<readonly PublicArticleTypeView[]> {
      const query = filter?.trim()
      const base: Omit<NonNullable<ListPublicArticleTypesData["query"]>, "page"> = { pageSize: PUBLIC_PAGE_SIZE, ...(query === undefined || query.length === 0 ? {} : { q: query }) }
      const first = articleTypePage((await client.get(PUBLIC_ARTICLE_TYPE_ROUTE, { params: { ...base, page: 1 } })).data, serverUrl)
      const items = [...first.items]
      for (let page = 2; page <= first.totalPages; page += 1) items.push(...articleTypePage((await client.get(PUBLIC_ARTICLE_TYPE_ROUTE, { params: { ...base, page } })).data, serverUrl).items)
      return items
    },
    async listTags(filter?: string): Promise<readonly PublicTagView[]> {
      const query = filter?.trim()
      const base: Omit<NonNullable<ListPublicTagsData["query"]>, "page"> = { pageSize: PUBLIC_PAGE_SIZE, ...(query === undefined || query.length === 0 ? {} : { q: query }) }
      const first = tagPage((await client.get(PUBLIC_TAG_ROUTE, { params: { ...base, page: 1 } })).data)
      const items = [...first.items]
      for (let page = 2; page <= first.totalPages; page += 1) items.push(...tagPage((await client.get(PUBLIC_TAG_ROUTE, { params: { ...base, page } })).data).items)
      return items
    },
  }
}
