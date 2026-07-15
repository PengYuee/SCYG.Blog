import type { MutationGuard } from "@/services/mutation-guard"
import type { AuthorArticleRepository, UploadedArticleImage } from "@/services/author-contracts"

/** 编辑器图片上传失败。 */
export class ImageUploadError extends Error {
  /** 稳定错误名称。 */ readonly name = "ImageUploadError"
  /** 创建保留原始原因的上传错误。 */
  constructor(cause: unknown) { super("图片上传失败", { cause }) }
}

/** 编辑器图片资源生命周期。 */
export type ImageLifecycle = {
  /** 上传成功后返回可插入 Markdown 的远程地址。 */ readonly upload: (file: File) => Promise<string>
  /** 标记失败批次中的成功地址，使文章提交后仍可取消。 */ readonly retainForCancel: (urls: readonly string[]) => void
  /** 保存成功后将临时远程地址标记为已提交。 */ readonly commit: () => void
  /** 取消编辑并返回全部远程资源是否已释放。 */ readonly cancel: () => Promise<boolean>
  /** 创建受跟踪的本地预览地址。 */ readonly preview: (file: File) => string
}

/** 创建 blob -> pending -> uploaded -> committed 生命周期。 */
export function createImageLifecycle(repository: AuthorArticleRepository, guard: MutationGuard): ImageLifecycle {
  /** 本地对象地址集合；取消时逐一释放。 */
  const localUrls = new Set<string>()
  /** 每个文件独占的本地预览地址，上传成功只释放自己的预览。 */
  const previewByFile = new WeakMap<File, string>()
  /** 已上传但尚未随文章提交的结构化远程资源。 */
  const pendingImages = new Map<string, UploadedArticleImage>()
  /** 未插入 Markdown 的部分成功资源，文章提交后仍等待取消。 */
  const retainedImageIds = new Set<string>()
  /** 释放当前编辑会话创建的全部本地对象地址。 */
  const revokeLocalUrls = (): void => {
    for (const url of localUrls) URL.revokeObjectURL(url)
    localUrls.clear()
  }
  /** 释放指定上传文件拥有的预览地址。 */
  const revokeFilePreview = (file: File): void => {
    const url = previewByFile.get(file)
    if (url === undefined) return
    URL.revokeObjectURL(url)
    localUrls.delete(url)
    previewByFile.delete(file)
  }
  return {
    preview(file) {
      const url = URL.createObjectURL(file)
      localUrls.add(url)
      previewByFile.set(file, url)
      return url
    },
    async upload(file) {
      try {
        const result = await guard.execute("image", () => repository.uploadImage(file))
        if (!result.ok) throw new ImageUploadError(result.error)
        if (result.value.url.startsWith("data:")) throw new ImageUploadError("data URL rejected")
        revokeFilePreview(file)
        pendingImages.set(result.value.id, result.value)
        return result.value.url
      } catch (error) {
        if (error instanceof ImageUploadError) throw error
        throw new ImageUploadError(error)
      }
    },
    retainForCancel(urls) {
      const retainedUrls = new Set(urls)
      for (const image of pendingImages.values()) if (retainedUrls.has(image.url)) retainedImageIds.add(image.id)
    },
    commit() {
      for (const id of pendingImages.keys()) if (!retainedImageIds.has(id)) pendingImages.delete(id)
    },
    async cancel() {
      revokeLocalUrls()
      const pending = [...pendingImages.values()]
      pendingImages.clear()
      retainedImageIds.clear()
      const outcomes = await Promise.allSettled(pending.map((image) => guard.execute("image", () => repository.deleteImage(image.id))))
      return outcomes.every((outcome) => outcome.status === "fulfilled" && outcome.value.ok && outcome.value.value)
    },
  }
}
