import { describe, expect, it, vi } from "vitest"
import { createImageLifecycle, ImageUploadError } from "@/services/image-lifecycle"
import type { AuthorArticleRepository } from "@/services/author-contracts"
import type { MutationGuard } from "@/services/mutation-guard"

function repository(upload: (file: File) => Promise<string>, remove: (name: string) => Promise<boolean>): AuthorArticleRepository {
  return { async detail() { throw new ImageUploadError("unused") }, async create() { return true }, async update() { return true }, uploadImage: upload, deleteImage: remove }
}
const guard: MutationGuard = { async execute(_domain, operation) { return { ok: true, value: await operation() } } }
const blockedGuard: MutationGuard = { async execute(domain, _operation) { return { ok: false, error: { code: "MUTATION_BLOCKED", domain, reason: "production blocked" } } } }

describe("image lifecycle", () => {
  it("deletes only unsaved uploaded images on cancel", async () => {
    // Given: 成功上传的临时远程图片。
    const remove = vi.fn(async () => true)
    const lifecycle = createImageLifecycle(repository(async () => "https://fake.local/images/pending.png", remove), guard)
    await lifecycle.upload(new File(["x"], "pending.png"))
    // When: 用户取消编辑。
    await lifecycle.cancel()
    // Then: 临时远程图片被删除。
    expect(remove).toHaveBeenCalledWith("pending.png")
  })
  it("does not delete committed image on later cleanup", async () => {
    // Given: 上传后已随文章提交的图片。
    const remove = vi.fn(async () => true)
    const lifecycle = createImageLifecycle(repository(async () => "https://fake.local/images/kept.png", remove), guard)
    await lifecycle.upload(new File(["x"], "kept.png")); lifecycle.commit()
    // When: 页面卸载执行清理。
    await lifecycle.cancel()
    // Then: 已提交远程图片不被删除。
    expect(remove).not.toHaveBeenCalled()
  })
  it("keeps markdown insertion callback unreachable when upload fails", async () => {
    // Given: 图片仓储上传失败。
    const lifecycle = createImageLifecycle(repository(async () => { throw new ImageUploadError("failed") }, async () => true), guard)
    // When / Then: 生命周期拒绝上传，不会产生可插入 Markdown 的地址。
    await expect(lifecycle.upload(new File(["x"], "failed.png"))).rejects.toBeInstanceOf(ImageUploadError)
  })
  it("makes zero repository calls when the production guard blocks upload", async () => {
    // Given: 生产写守卫与可观测上传仓储。
    const upload = vi.fn(async () => "https://fake.local/images/blocked.png")
    const lifecycle = createImageLifecycle(repository(upload, async () => true), blockedGuard)
    // When: 编辑器尝试上传。
    await expect(lifecycle.upload(new File(["x"], "blocked.png"))).rejects.toBeInstanceOf(ImageUploadError)
    // Then: 守卫在适配器调用前拒绝，网络仓储调用数为零。
    expect(upload).not.toHaveBeenCalled()
  })
  it("releases only the preview owned by the successful upload", async () => {
    // Given: 两个文件分别拥有本地预览。
    const first = new File(["a"], "first.png")
    const second = new File(["b"], "second.png")
    const revoke = vi.spyOn(URL, "revokeObjectURL")
    const create = vi.spyOn(URL, "createObjectURL").mockReturnValueOnce("blob:first").mockReturnValueOnce("blob:second")
    const lifecycle = createImageLifecycle(repository(async (file) => `/images/${file.name}`, async () => true), guard)
    lifecycle.preview(first); lifecycle.preview(second)
    // When: 仅第一个文件上传成功。
    await lifecycle.upload(first)
    // Then: 不相关的第二个预览仍由取消流程持有。
    expect(revoke).toHaveBeenCalledWith("blob:first")
    expect(revoke).not.toHaveBeenCalledWith("blob:second")
    await lifecycle.cancel()
    expect(revoke).toHaveBeenCalledTimes(2)
    create.mockRestore(); revoke.mockRestore()
  })
  it("cleans every relative temporary URL even when one delete rejects", async () => {
    // Given: 两个相对上传地址，首个删除失败。
    const remove = vi.fn(async (name: string) => { if (name === "first.png") throw new ImageUploadError("delete failed"); return true })
    let uploadCount = 0
    const upload = vi.fn(async (_file: File) => { uploadCount += 1; return uploadCount === 1 ? "/images/first.png?draft=1" : "images/second.png#pending" })
    const lifecycle = createImageLifecycle(repository(upload, remove), guard)
    await lifecycle.upload(new File(["a"], "first.png")); await lifecycle.upload(new File(["b"], "second.png"))
    // When: 取消清理全部临时地址。
    await lifecycle.cancel()
    // Then: 相对地址安全提取文件名，单个失败不阻断后续删除。
    expect(remove).toHaveBeenCalledWith("first.png")
    expect(remove).toHaveBeenCalledWith("second.png")
  })
})
