import type { MutationGuard } from "@/services/mutation-guard"
import type { AuthorArticleRepository } from "@/services/author-contracts"

/** 编辑器图片上传失败。 */
export class ImageUploadError extends Error {
  /** 稳定错误名称。 */ readonly name = "ImageUploadError"
  /** 创建保留原始原因的上传错误。 */
  constructor(cause: unknown) { super("图片上传失败", { cause }) }
}

/** 编辑器图片资源生命周期。 */
export type ImageLifecycle = {
  /** 上传成功后返回可插入 Markdown 的远程地址。 */ readonly upload: (file: File) => Promise<string>
  /** 保存成功后将临时远程地址标记为已提交。 */ readonly commit: () => void
  /** 取消编辑并释放全部本地及未提交远程资源。 */ readonly cancel: () => Promise<void>
  /** 创建受跟踪的本地预览地址。 */ readonly preview: (file: File) => string
}

/** 创建 blob -> pending -> uploaded -> committed 生命周期。 */
export function createImageLifecycle(repository: AuthorArticleRepository, guard: MutationGuard): ImageLifecycle {
  /** 本地对象地址集合；取消时逐一释放。 */
  const localUrls = new Set<string>()
  /** 已上传但尚未随文章提交的远程地址集合。 */
  const temporaryRemoteUrls = new Set<string>()
  /** 释放当前编辑会话创建的全部本地对象地址。 */
  const revokeLocalUrls = (): void => {
    for (const url of localUrls) URL.revokeObjectURL(url)
    localUrls.clear()
  }
  return {
    preview(file) {
      const url = URL.createObjectURL(file)
      localUrls.add(url)
      return url
    },
    async upload(file) {
      try {
        const result = await guard.execute("image", () => repository.uploadImage(file))
        if (!result.ok) throw new ImageUploadError(result.error)
        if (result.value.startsWith("data:")) throw new ImageUploadError("data URL rejected")
        revokeLocalUrls()
        temporaryRemoteUrls.add(result.value)
        return result.value
      } catch (error) {
        if (error instanceof ImageUploadError) throw error
        throw new ImageUploadError(error)
      }
    },
    commit() { temporaryRemoteUrls.clear() },
    async cancel() {
      revokeLocalUrls()
      for (const url of temporaryRemoteUrls) {
        const imageName = new URL(url).pathname.split("/").filter(Boolean).at(-1)
        if (imageName !== undefined) await guard.execute("image", () => repository.deleteImage(imageName))
      }
      temporaryRemoteUrls.clear()
    },
  }
}
