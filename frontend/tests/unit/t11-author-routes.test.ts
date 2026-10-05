import { describe, expect, it } from "vitest"
import { authorRoutes } from "@/router/modules/author"

describe("author route authentication", () => {
  it("registers the author workspace behind one parent authentication boundary", () => {
    const root = authorRoutes[0]
    expect(root?.path).toBe("/author")
    expect(root?.meta?.["requiresAuth"]).toBe(true)
    expect(root?.children?.map((route) => route.name)).toEqual(["author-article-new", "author-article-edit", "author-taxonomy"])
  })
})
