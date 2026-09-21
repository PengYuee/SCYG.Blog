import type { HttpTransport } from "@/request/transport"
import type { UploadedArticleImage } from "@/services/author-contracts"
import { normalizeImageUrl, parseBoundary } from "@/types/api"
import { articleImageSchema } from "./schemas"

/** 创建正文图片上传与取消 API 适配器。 */
export function createArticleImageApi(client: HttpTransport, serverUrl: string) {
  return {
    /** 以浏览器生成 boundary 的 multipart 表单上传单个文件。 */
    async uploadImage(image: File): Promise<UploadedArticleImage> {
      const form = new FormData()
      form.append("file", image)
      const response = await client.post("/api/v1/manage/article-images", form)
      const value = parseBoundary(articleImageSchema, response.data, "article image upload")
      return { id: value.id, url: normalizeImageUrl(value.url, serverUrl), expiresAt: value.expiresAt }
    },
    /** 严格按服务端图片标识取消待提交图片。 */
    async deleteImage(id: string): Promise<boolean> {
      await client.delete(`/api/v1/article-images/${id}`)
      return true
    },
  }
}
