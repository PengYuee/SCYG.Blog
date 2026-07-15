import { describe, expect, it, vi } from "vitest"
import { createArticleImageApi } from "@/request/api/article-image"
import type { HttpTransport } from "@/request/transport"

/** 创建隔离的类型化传输层替身。 */
function client(): HttpTransport {
  return { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() }
}

describe("article image API adapter", () => {
  it("uploads with browser-owned multipart headers and deletes by response id", async () => {
    // Given: 图片接口返回与 URL basename 不同的稳定资源标识。
    const transport = client()
    vi.mocked(transport.post).mockResolvedValue({ data: {
      id: "0123456789abcdef0123456789abcdef",
      storageKey: "ffffffffffffffffffffffffffffffff.png",
      url: "/media/article-images/ffffffffffffffffffffffffffffffff.png",
      mediaType: "png",
      byteSize: 4,
      width: 1,
      height: 1,
      status: "pending",
      expiresAt: "2026-07-14T00:00:00Z",
    } })
    vi.mocked(transport.delete).mockResolvedValue({ data: undefined })
    const api = createArticleImageApi(transport, "https://api.test")
    const file = new File(["png"], "正文.png", { type: "image/png" })

    // When: 上传后按返回标识取消图片。
    const uploaded = await api.uploadImage(file)
    await api.deleteImage(uploaded.id)

    // Then: FormData 不携带手工 Content-Type，删除也不从 URL 推导标识。
    const postCall = vi.mocked(transport.post).mock.calls[0]
    expect(postCall?.[0]).toBe("/api/v1/article-images")
    expect(postCall?.[1]).toBeInstanceOf(FormData)
    expect(postCall).toHaveLength(2)
    expect(uploaded).toEqual({ id: "0123456789abcdef0123456789abcdef", url: "https://api.test/media/article-images/ffffffffffffffffffffffffffffffff.png", expiresAt: "2026-07-14T00:00:00Z" })
    expect(transport.delete).toHaveBeenCalledWith("/api/v1/article-images/0123456789abcdef0123456789abcdef")
  })
})
