import { describe, expect, it } from "vitest"
import { createAuthorRoutes } from "@/router/modules/author"

describe("T11 author route availability", () => {
  it("uses available metadata for every explicit fake route", () => {
    // Given: 显式启用 Fake 的作者路由。
    const root = createAuthorRoutes(true)[0]
    // When / Then: 根与所有子页面都声明 available。
    expect(root?.meta?.["availability"]).toEqual({ kind: "available" })
    for (const route of root?.children ?? []) expect(route.meta?.["availability"]).toEqual({ kind: "available" })
  })
  it("keeps production metadata unavailable", () => {
    // Given: 生产作者路由。
    const root = createAuthorRoutes(false)[0]
    // When / Then: 根与所有子页面保持真实不可用。
    expect(root?.meta?.["availability"]).toMatchObject({ kind: "unavailable" })
    for (const route of root?.children ?? []) expect(route.meta?.["availability"]).toMatchObject({ kind: "unavailable" })
  })
})
