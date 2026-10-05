import { START_LOCATION } from "vue-router"
import { describe, expect, it, vi } from "vitest"
import { createBlogScrollBehavior } from "@/router/scroll-behavior"

describe("router scroll priority", () => {
  it("prioritizes an eligible initial reload over a non-null history saved position", async () => {
    const restoration = { arm: vi.fn(() => true), cancel: vi.fn(), wait: vi.fn(async () => false) }
    const behavior = createBlogScrollBehavior(restoration)
    expect(await behavior({ hash: "", fullPath: "/articles", name: "article-list" }, START_LOCATION, { left: 0, top: 88 })).toBe(false)
    expect(restoration.arm).toHaveBeenCalledWith("/articles")
    expect(restoration.wait).toHaveBeenCalledOnce()
  })

  it("keeps saved position first for ordinary back and forward navigation", async () => {
    const restoration = { arm: vi.fn(() => false), cancel: vi.fn(), wait: vi.fn(async () => false) }
    const behavior = createBlogScrollBehavior(restoration)
    expect(await behavior({ hash: "", fullPath: "/articles", name: "article-list" }, { fullPath: "/articles/1" }, { left: 0, top: 420 })).toEqual({ left: 0, top: 420 })
    expect(restoration.cancel).toHaveBeenCalledOnce()
    expect(restoration.arm).not.toHaveBeenCalled()
  })

  it("cancels pending reload restoration on every later navigation before hash or top", async () => {
    const restoration = { arm: vi.fn(() => false), cancel: vi.fn(), wait: vi.fn(async () => false) }
    const behavior = createBlogScrollBehavior(restoration)
    expect(await behavior({ hash: "#content", fullPath: "/next#content", name: "article-list" }, { fullPath: "/" }, null)).toEqual({ el: "#content" })
    expect(await behavior({ hash: "", fullPath: "/next", name: "article-list" }, { fullPath: "/" }, null)).toEqual({ top: 0 })
    expect(restoration.cancel).toHaveBeenCalledTimes(2)
  })

  it.each([
    ["admin-unavailable", "/admin"],
    ["public-not-found", "/missing"],
    ["login", "/login"],
    ["author-article-new", "/author/articles/new"],
    ["legacy-article-invalid", "/article/nope"],
  ])("does not arm initial route %s because it has no ready signal", async (name, fullPath) => {
    const restoration = { arm: vi.fn(() => true), cancel: vi.fn(), wait: vi.fn(async () => false) }
    const behavior = createBlogScrollBehavior(restoration)
    expect(await behavior({ hash: "", fullPath, name }, START_LOCATION, null)).toEqual({ top: 0 })
    expect(restoration.arm).not.toHaveBeenCalled()
  })
})
