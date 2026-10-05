import type { CreateManageArticleImageData, DeleteArticleImageData } from "@/request/generated"
import { zArticleImage } from "@/request/generated/zod.gen"
import type { HttpTransport } from "@/request/transport"
import type { UploadedArticleImage } from "@/services/author-contracts"
import { normalizeImageUrl, parseBoundary } from "@/types/api"

const ARTICLE_IMAGE_UPLOAD_ROUTE: CreateManageArticleImageData["url"] = "/api/v1/manage/article-images"
const ARTICLE_IMAGE_DELETE_ROUTE: DeleteArticleImageData["url"] = "/api/v1/article-images/{imageId}"

/** 正文图片 API 适配器契约。 */
export interface ArticleImageApi {
  readonly uploadImage: (image: File) => Promise<UploadedArticleImage>
  readonly deleteImage: (id: string) => Promise<boolean>
}

/** 创建正文图片上传与取消 API 适配器。 */
export function createArticleImageApi(client: HttpTransport, serverUrl: string): ArticleImageApi {
  return {
    /** 以浏览器生成 boundary 的 multipart 表单上传单个文件。 */
    async uploadImage(image: File): Promise<UploadedArticleImage> {
      const body: CreateManageArticleImageData["body"] = { file: image }
      const form = new FormData()
      form.append("file", body.file)
      const response = await client.post(ARTICLE_IMAGE_UPLOAD_ROUTE, form)
      const value = parseBoundary(zArticleImage, response.data, "article image upload")
      return { id: value.id, url: normalizeImageUrl(value.url, serverUrl), expiresAt: value.expiresAt }
    },
    /** 严格按服务端图片标识取消待提交图片。 */
    async deleteImage(id: string): Promise<boolean> {
      const path: DeleteArticleImageData["path"] = { imageId: id }
      await client.delete(ARTICLE_IMAGE_DELETE_ROUTE.replace("{imageId}", path.imageId))
      return true
    },
  }
}
