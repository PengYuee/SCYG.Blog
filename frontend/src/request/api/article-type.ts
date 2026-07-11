import type { AxiosInstance } from "axios"
import { z } from "zod"
import type { ArticleType, ArticleTypeCreate } from "@/types/taxonomy"
import { normalizeImageUrl, parseBoundary } from "@/types/api"
import { articleTypeSchema, mutationSchema, pageSchema } from "./schemas"

const dictionarySchema = z.strictObject({ items: z.array(articleTypeSchema), page: pageSchema })

/** 解析当前分类字典包络。 */
export function parseArticleTypes(input: unknown, serverUrl: string): readonly ArticleType[] {
  return parseBoundary(dictionarySchema, input, "article type dictionary").items.map((item) => ({ id: item.id, name: item.name, imageUrl: item.image === null ? null : normalizeImageUrl(item.image, serverUrl), menu: item.meun }))
}

/** 旧分类 API 的类型化适配器。 */
export function createArticleTypeApi(client: AxiosInstance, serverUrl: string) {
  return {
    /** 获取分类字典。 */ async list(filter?: string) { const response = await client.get("/ArticleType/GetArticleTypeDic", { params: filter === undefined ? {} : { filter } }); return parseArticleTypes(response.data, serverUrl) },
    /** 创建分类，沿用旧接口 FormData。 */ async create(request: ArticleTypeCreate) { const form = new FormData(); form.append("name", request.name); form.append("meun", String(request.menu)); if (request.image !== null) form.append("image", request.image); const response = await client.post("/ArticleType/CreateArticleType", form); return parseBoundary(mutationSchema, response.data, "article type create") },
    /** 删除分类。 */ async delete(id: number) { const response = await client.delete("/ArticleType/DeleteArticleType", { params: { id } }); return parseBoundary(mutationSchema, response.data, "article type delete") },
  }
}
