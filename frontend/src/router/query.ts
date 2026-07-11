import { z } from "zod"
import type { LocationQuery, LocationQueryRaw } from "vue-router"

/** 文章列表规范查询。 */
export type ArticleListQuery = {
  /** 已去除首尾空白的本地搜索词。 */ readonly q: string
  /** 可选分类标识。 */ readonly categoryId?: number | undefined
  /** 可选标签标识。 */ readonly tagId?: number | undefined
}

/** URL 查询解析结果。 */
export type ArticleListQueryResult =
  | { readonly kind: "valid"; readonly value: ArticleListQuery }
  | { readonly kind: "invalid"; readonly code: "INVALID_ARTICLE_QUERY"; readonly message: string }

const optionalId = z.string().regex(/^[1-9]\d*$/).transform(Number).optional()
const querySchema = z.strictObject({ q: z.string().trim().default(""), categoryId: optionalId, tagId: optionalId })

/** 在路由边界解析 q/categoryId/tagId，拒绝重复或非法值。 */
export function parseArticleListQuery(query: LocationQuery | Readonly<Record<string, unknown>>): ArticleListQueryResult {
  const result = querySchema.safeParse(query)
  if (!result.success) return { kind: "invalid", code: "INVALID_ARTICLE_QUERY", message: "筛选参数无效，请检查分类或标签标识。" }
  return { kind: "valid", value: result.data }
}

/** 以 q/categoryId/tagId 固定顺序序列化可分享查询。 */
export function serializeArticleListQuery(query: ArticleListQuery): LocationQueryRaw {
  const result: LocationQueryRaw = {}
  const normalizedQuery = query.q.trim()
  if (normalizedQuery.length > 0) result["q"] = normalizedQuery
  if (query.categoryId !== undefined) result["categoryId"] = String(query.categoryId)
  if (query.tagId !== undefined) result["tagId"] = String(query.tagId)
  return result
}

/** 将单值 URL 字段解析为正整数，供旧路由兼容层复用。 */
export function parseLegacyArticleId(value: unknown): number | null {
  const result = optionalId.safeParse(value)
  return result.success && result.data !== undefined ? result.data : null
}
