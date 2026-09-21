import { mount, type VueWrapper } from "@vue/test-utils"
import { createMemoryHistory, createRouter, type Router } from "vue-router"
import { afterEach, describe, expect, it, vi } from "vitest"

const homePath = "/"
const articlesPath = "/articles"
const authorWritePath = "/author/articles/new"
const TestRouteView = { template: "<div />" }

/** 创建仅覆盖公共导航目标的内存路由。 */
async function createNavigationRouter(): Promise<Router> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: homePath, component: TestRouteView },
      { path: articlesPath, component: TestRouteView },
      { path: authorWritePath, component: TestRouteView },
    ],
  })
  await router.push(homePath)
  await router.isReady()
  return router
}

/** 通过在组件导入前注入作者开关，隔离静态导航目标的模块求值。 */
async function mountBlogNavigation(fakeAuthorEnabled: boolean): Promise<VueWrapper> {
  vi.resetModules()
  vi.doMock("@/services/author-runtime", () => ({ fakeAuthorEnabled }))
  const { default: BlogNavigation } = await import("@/components/public/BlogNavigation.vue")
  const router = await createNavigationRouter()
  return mount(BlogNavigation, { global: { plugins: [router] } })
}

afterEach(() => {
  vi.doUnmock("@/services/author-runtime")
  vi.resetModules()
})

describe("BlogNavigation 作者写作入口", () => {
  it("在 Fake 作者模式启用时渲染写作台链接", async () => {
    // Given: 作者运行时开关在组件导入前被确定为启用。
    const wrapper = await mountBlogNavigation(true)

    // When: 公共导航通过 Vue Router 挂载。
    const authorWriteLink = wrapper.get(`a[href="${authorWritePath}"]`)

    // Then: 用户可见写作台，并能进入规范新建文章路径。
    expect(authorWriteLink.text()).toBe("写作台")
    expect(authorWriteLink.attributes("href")).toBe(authorWritePath)
  })

  it("在 Fake 作者模式禁用时隐藏写作台并保留公共导航", async () => {
    // Given: 作者运行时开关在组件导入前被确定为禁用。
    const wrapper = await mountBlogNavigation(false)

    // When: 公共导航通过 Vue Router 挂载。
    const authorWriteLink = wrapper.find(`a[href="${authorWritePath}"]`)

    // Then: 写作台不可见，首页与文章仍是可访问的公共入口。
    expect(authorWriteLink.exists()).toBe(false)
    expect(wrapper.get(`a[href="${homePath}"]`).text()).toBe("首页")
    expect(wrapper.get(`a[href="${articlesPath}"]`).text()).toBe("文章")
  })
})
