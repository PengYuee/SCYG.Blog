import { describe, expect, it, vi } from "vitest"
import { createImageLifecycle, ImageUploadError } from "@/services/image-lifecycle"
import { createFakeAuthorRepositories } from "@/services/fake-author"
import type { AuthorArticleRepository, UploadedArticleImage } from "@/services/author-contracts"
import type { MutationGuard } from "@/services/mutation-guard"

/** 创建结构化待提交图片 fixture。 */
function uploaded(id: string, url: string): UploadedArticleImage {
  return { id, url, expiresAt: "2026-07-14T00:00:00Z" }
}

/** 创建只覆盖图片行为的作者仓储替身。 */
function repository(upload: (file: File) => Promise<UploadedArticleImage>, remove: (id: string) => Promise<boolean>): AuthorArticleRepository {
  return { async detail() { throw new ImageUploadError("unused") }, async create() { return true }, async update() { return true }, uploadImage: upload, deleteImage: remove }
}
const guard: MutationGuard = { async execute(_domain, operation) { return { ok: true, value: await operation() } } }
const blockedGuard: MutationGuard = { async execute(domain, _operation) { return { ok: false, error: { code: "MUTATION_BLOCKED", domain, reason: "production blocked" } } } }

describe("image lifecycle", () => {
  it("characterizes the existing fake upload and cancel lifecycle", async () => {
    // Given: 现有 Fake 作者仓储与受信写守卫。
    const fake = createFakeAuthorRepositories()
    const lifecycle = createImageLifecycle(fake.articles, guard)
    // When: 上传一张图片后取消当前编辑会话。
    await lifecycle.upload(new File(["fake"], "characterization.png", { type: "image/png" }))
    await lifecycle.cancel()
    // Then: Fake 仍记录一次上传与一次远程取消。
    expect(fake.calls.uploads).toBe(1)
    expect(fake.calls.imageDeletes).toBe(1)
  })

  it("deletes only unsaved uploaded images on cancel", async () => {
    // Given: 成功上传的临时远程图片。
    const remove = vi.fn(async () => true)
    const lifecycle = createImageLifecycle(repository(async () => uploaded("pending-id", "https://fake.local/images/not-the-id.png"), remove), guard)
    await lifecycle.upload(new File(["x"], "pending.png"))
    // When: 用户取消编辑。
    await lifecycle.cancel()
    // Then: 临时远程图片被删除。
    expect(remove).toHaveBeenCalledWith("pending-id")
  })
  it("does not delete committed image on later cleanup", async () => {
    // Given: 上传后已随文章提交的图片。
    const remove = vi.fn(async () => true)
    const lifecycle = createImageLifecycle(repository(async () => uploaded("kept-id", "https://fake.local/images/kept.png"), remove), guard)
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
  it("continues rejecting data URLs returned by an adapter", async () => {
    // Given: 越过仓储边界返回 Base64 地址的异常适配器。
    const remove = vi.fn(async () => true)
    const lifecycle = createImageLifecycle(repository(async () => uploaded("data-id", "data:image/png;base64,eA=="), remove), guard)
    // When / Then: 上传被拒绝且不会登记为可取消远程资源。
    await expect(lifecycle.upload(new File(["x"], "data.png"))).rejects.toBeInstanceOf(ImageUploadError)
    await lifecycle.cancel()
    expect(remove).not.toHaveBeenCalled()
  })
  it("makes zero repository calls when the production guard blocks upload", async () => {
    // Given: 生产写守卫与可观测上传仓储。
    const upload = vi.fn(async () => uploaded("blocked-id", "https://fake.local/images/blocked.png"))
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
    const lifecycle = createImageLifecycle(repository(async (file) => uploaded(file.name, `/images/${file.name}`), async () => true), guard)
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
  it("cleans every structured pending id even when one delete rejects", async () => {
    // Given: 两个结构化待提交资源，首个删除失败。
    const remove = vi.fn(async (id: string) => { if (id === "first-id") throw new ImageUploadError("delete failed"); return true })
    let uploadCount = 0
    const upload = vi.fn(async (_file: File) => { uploadCount += 1; return uploadCount === 1 ? uploaded("first-id", "/images/unrelated-a.png") : uploaded("second-id", "/images/unrelated-b.png") })
    const lifecycle = createImageLifecycle(repository(upload, remove), guard)
    await lifecycle.upload(new File(["a"], "first.png")); await lifecycle.upload(new File(["b"], "second.png"))
    // When: 取消清理全部临时地址。
    const cleaned = await lifecycle.cancel()
    // Then: 单个失败不阻断其他标识删除，TTL 可兜底首个失败项。
    expect(remove).toHaveBeenCalledWith("first-id")
    expect(remove).toHaveBeenCalledWith("second-id")
    expect(cleaned).toBe(false)
  })
  it("keeps a successful pending image cancellable when a sibling upload fails", async () => {
    // Given: 第一张成功、第二张失败的多图上传。
    const remove = vi.fn(async () => true)
    let uploadCount = 0
    const lifecycle = createImageLifecycle(repository(async () => {
      uploadCount += 1
      if (uploadCount === 2) throw new ImageUploadError("second failed")
      return uploaded("successful-id", "/images/successful.png")
    }, remove), guard)
    // When: 部分上传失败后取消会话。
    const successfulUrl = await lifecycle.upload(new File(["a"], "first.png"))
    await expect(lifecycle.upload(new File(["b"], "second.png"))).rejects.toBeInstanceOf(ImageUploadError)
    lifecycle.retainForCancel([successfulUrl])
    lifecycle.commit()
    await lifecycle.cancel()
    // Then: 即使文章保存执行 commit，失败批次成功项仍按独立 id 取消。
    expect(remove).toHaveBeenCalledWith("successful-id")
  })
})
