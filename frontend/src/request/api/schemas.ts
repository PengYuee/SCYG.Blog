import { z } from "zod"

/** 当前 API 分页 schema。 */
export const pageSchema = z.strictObject({ number: z.number().int().positive(), size: z.number().int().positive(), total_items: z.number().int().nonnegative(), total_pages: z.number().int().nonnegative() })

/** 当前 API 文章 schema。 */
export const articleSchema = z.strictObject({
  id: z.number().int().positive(), title: z.string().min(1), slug: z.string().min(1), digest: z.string().min(1), content: z.string().min(1),
  article_type_id: z.number().int().positive(), tag_ids: z.array(z.number().int().positive()), status: z.union([z.literal(1), z.literal(2), z.literal(3)]),
  support: z.number().int().nonnegative(), comment: z.number().int().nonnegative(), visited: z.number().int().nonnegative(), version: z.number().int().positive(),
  created_at: z.iso.datetime(), updated_at: z.iso.datetime().nullable(),
})

/** 当前 API 分类 schema。 */
export const articleTypeSchema = z.strictObject({ id: z.number().int().positive(), name: z.string().min(1), image: z.string().nullable(), meun: z.number().int().nonnegative(), version: z.number().int().positive(), created_at: z.iso.datetime(), updated_at: z.iso.datetime().nullable() })

/** 当前 API 标签 schema。 */
export const tagSchema = z.strictObject({ id: z.number().int().positive(), name: z.string().min(1), version: z.number().int().positive(), created_at: z.iso.datetime(), updated_at: z.iso.datetime().nullable() })

/** 旧接口通用变更结果 schema。 */
export const mutationSchema = z.union([z.strictObject({ success: z.literal(true) }), z.boolean()])
