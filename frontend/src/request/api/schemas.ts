import { z } from "zod"

/** 当前 API 分页 schema。 */
export const pageSchema = z.strictObject({
  number: z.number().int().positive(),
  size: z.number().int().min(1).max(100),
  total_items: z.number().int().nonnegative(),
  total_pages: z.number().int().nonnegative(),
})

const positiveIdSchema = z.number().int().positive()
const uniquePositiveIdsSchema = z.array(positiveIdSchema).refine((ids) => new Set(ids).size === ids.length, { message: "tag_ids must contain unique values" })

/** 当前 API 文章 schema。 */
export const articleSchema = z.strictObject({
  id: positiveIdSchema,
  title: z.string().min(1).max(120),
  slug: z.string().min(1).max(160).regex(/^[a-z0-9]+(?:-[a-z0-9]+)*$/),
  digest: z.string().min(1).max(500),
  content: z.string().min(1),
  article_type_id: positiveIdSchema,
  tag_ids: uniquePositiveIdsSchema,
  status: z.union([z.literal(1), z.literal(2), z.literal(3)]),
  support: z.number().int().nonnegative(),
  comment: z.number().int().nonnegative(),
  visited: z.number().int().nonnegative(),
  version: z.number().int().positive(),
  created_at: z.iso.datetime(),
  updated_at: z.iso.datetime().nullable(),
})

/** 当前 API 分类 schema。 */
export const articleTypeSchema = z.strictObject({
  id: positiveIdSchema,
  name: z.string().min(1).max(60),
  image: z.string().max(512).nullable(),
  meun: z.number().int().nonnegative(),
  version: z.number().int().positive(),
  created_at: z.iso.datetime(),
  updated_at: z.iso.datetime().nullable(),
})

/** 当前 API 标签 schema。 */
export const tagSchema = z.strictObject({
  id: positiveIdSchema,
  name: z.string().min(1).max(60),
  version: z.number().int().positive(),
  created_at: z.iso.datetime(),
  updated_at: z.iso.datetime().nullable(),
})

/** 当前 API 正文图片 schema。 */
export const articleImageSchema = z.strictObject({
  id: z.string().regex(/^[0-9a-f]{32}$/),
  storageKey: z.string().regex(/^[0-9a-f]{32}\.(?:jpg|png)$/),
  url: z.string().regex(/^\/media\/article-images\/[0-9a-f]{32}\.(?:jpg|png)$/),
  mediaType: z.enum(["jpeg", "png"]),
  byteSize: z.number().int().min(1).max(5_242_880),
  width: z.number().int().min(1).max(8192),
  height: z.number().int().min(1).max(8192),
  status: z.literal("pending"),
  expiresAt: z.iso.datetime(),
})
