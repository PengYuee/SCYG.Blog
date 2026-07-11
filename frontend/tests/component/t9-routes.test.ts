import { flushPromises, mount } from "@vue/test-utils"
import { createPinia } from "pinia"
import { RouterView } from "vue-router"
import { describe, expect, it } from "vitest"
import { router } from "@/router"
import { adminRoutes } from "@/router/modules/admin"
import { authorRoutes } from "@/router/modules/author"
import { publicRoutes } from "@/router/modules/public"

/** 导航共享生产路由并等待懒加载组件稳定。 */
async function navigate(path: string): Promise<void> {
  await router.push(path)
  await router.isReady()
  await flushPromises()
}

describe("T9 production route contract", () => {
  it.each([
    ["/", "home"],
    ["/articles", "article-list"],
    ["/articles/42", "article-detail"],
    ["/login", "login-unavailable"],
  ])("resolves public deep link %s", (path, routeName) => {
    // Given / When: 路由归属只需同步解析，不需要触发懒加载视图。
    const resolvedRoute = router.resolve(path)
    // Then: 深链由明确公共路由拥有。
    expect(resolvedRoute.name).toBe(routeName)
  })

  it.each([
    ["/main", "/"],
    ["/article/42", "/articles/42"],
    ["/writeBlog", "/author/articles/new"],
    ["/writeBlog?id=42", "/author/articles/42/edit"],
  ])("redirects exact legacy URL %s to %s", async (legacy, expected) => {
    // Given / When: 旧书签被生产路由解析。
    await navigate(legacy)
    // Then: 跳转落到唯一规范地址。
    expect(router.currentRoute.value.fullPath).toBe(expected)
  })

  it("keeps an invalid legacy article id on a typed failure route", async () => {
    // Given / When: 旧文章链接携带非正整数标识。
    await navigate("/article/not-a-number")
    // Then: 地址保持不变并暴露稳定失败代码，不进入普通详情 404。
    expect(router.currentRoute.value.fullPath).toBe("/article/not-a-number")
    expect(router.currentRoute.value.name).toBe("legacy-article-invalid")
    expect(router.currentRoute.value.meta["errorCode"]).toBe("INVALID_ARTICLE_ID")
  })
  it("keeps an invalid legacy editor id on a typed failure route", async () => {
    // Given / When: 旧写作链接携带非法标识。
    await navigate("/writeBlog?id=oops")
    // Then: 地址不被静默改写，且暴露稳定失败代码。
    expect(router.currentRoute.value.name).toBe("legacy-write-invalid")
    expect(router.currentRoute.value.meta["errorCode"]).toBe("INVALID_ARTICLE_ID")
  })

  it("lets the admin module own every admin descendant before the public catch-all", async () => {
    // Given / When: 未实现的未来后台子路径被直接打开。
    await navigate("/admin/future/child")
    // Then: 后台不可用边界处理该路径，公共 404 不会吞掉它。
    expect(router.currentRoute.value.name).toBe("admin-unavailable-child")
    expect(router.currentRoute.value.matched.some(({ name }) => name === "public-not-found")).toBe(false)
  })

  it("shows the public 404 for an unknown non-admin path", async () => {
    // Given / When: 未知公共地址被直接打开。
    await navigate("/missing-page")
    // Then: 公共 catch-all 保留原地址并呈现 404。
    expect(router.currentRoute.value.name).toBe("public-not-found")
    expect(router.currentRoute.value.fullPath).toBe("/missing-page")
  })

  it("keeps admin matching independent from public and author route records", async () => {
    // Given: 生产路由的顶级匹配记录。
    const adminRecord = adminRoutes[0]
    // When: 模块边界被检查。
    const publicCatchAll = publicRoutes.find(({ name }) => name === "public-not-found")
    // Then: admin 自己拥有后代 catch-all，且 author/public 保持独立数组。
    expect(adminRecord?.children?.some(({ path }) => path === ":pathMatch(.*)*")).toBe(true)
    expect(publicCatchAll?.path).toBe("/:pathMatch(.*)*")
    expect(authorRoutes.every(({ path }) => path.startsWith("/author"))).toBe(true)
  })

  it("updates the document title from route metadata", async () => {
    // Given / When: 用户进入文章发现页。
    await navigate("/articles")
    // Then: 浏览器标题使用路由元数据与统一站点后缀。
    expect(document.title).toBe("文章 · SCYG Blog")
  })
  it("renders a deep-linked route through RouterView", async () => {
    // Given: 应用外壳挂载生产路由。
    await navigate("/admin/deep-link")
    const wrapper = mount(RouterView, { global: { plugins: [createPinia(), router] } })
    // When: 后台不可用懒加载页完成挂载。
    await flushPromises()
    // Then: 页面真实渲染，而非仅匹配记录。
    expect(wrapper.text()).toContain("管理后台暂不可用")
  })
})
