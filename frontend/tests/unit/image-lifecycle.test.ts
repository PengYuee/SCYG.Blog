import { describe, expect, it, vi } from "vitest"
import { createImageLifecycle, ImageUploadError } from "@/services/image-lifecycle"
import type { AuthorArticleRepository } from "@/services/author-contracts"
import type { MutationGuard } from "@/services/mutation-guard"

function repository(upload: () => Promise<string>, remove: (name: string) => Promise<boolean>): AuthorArticleRepository {
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
})
