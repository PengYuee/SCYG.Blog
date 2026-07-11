import type { AxiosInstance } from "axios"
import { z } from "zod"
import type { Tag } from "@/types/taxonomy"
import { parseBoundary } from "@/types/api"
import { mutationSchema, pageSchema, tagSchema } from "./schemas"

const dictionarySchema = z.strictObject({ items: z.array(tagSchema), page: pageSchema })

/** 解析当前标签字典包络。 */
export function parseTags(input: unknown): readonly Tag[] {
  return parseBoundary(dictionarySchema, input, "tag dictionary").items.map((item) => ({ id: item.id, name: item.name }))
}

/** 旧标签 API 的类型化适配器。 */
export function createTagApi(client: AxiosInstance) {
  return {
    /** 获取标签字典。 */ async list(filter?: string) { const response = await client.get("/Tag/GetTagDic", { params: filter === undefined ? {} : { filter } }); return parseTags(response.data) },
    /** 创建标签。 */ async create(name: string) { const response = await client.post("/Tag/CreateTag", { name }); return parseBoundary(mutationSchema, response.data, "tag create") },
    /** 删除标签。 */ async delete(id: number) { const response = await client.delete("/Tag/DeleteTag", { params: { id } }); return parseBoundary(mutationSchema, response.data, "tag delete") },
  }
}
