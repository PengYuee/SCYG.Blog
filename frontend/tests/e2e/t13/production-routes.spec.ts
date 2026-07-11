import { expect, test, type Locator, type Page } from "@playwright/test"
import { installMutationRecorder, installReadFixtures } from "./fixtures/api-fixtures"
import { expectHealthy, gotoReady, observePageHealth } from "./helpers/page-health"

/** 每个路由显式提供唯一就绪 Locator，禁止通用 selector 掩盖重复节点。 */
const publicRoutes: readonly { readonly path: string; readonly ready: (page: Page) => Locator }[] = [
  { path: "/", ready: (page) => page.locator("#hero-profile-name") },
  { path: "/articles", ready: (page) => page.getByRole("heading", { name: "文章归档", level: 1 }) },
  { path: "/articles/101", ready: (page) => page.getByRole("heading", { name: "在 Edge 中验证中文博客的完整阅读体验", level: 1 }) },
  { path: "/login", ready: (page) => page.getByRole("heading", { name: "登录功能暂不可用", level: 1 }) },
]

for (const route of publicRoutes) {
  test(`生产公共路由 ${route.path} 可读且无异常`, async ({ page }) => {
    // Given: 导航前安装全部真实读取 fixture，并观察生产写请求。
    const health = observePageHealth(page)
    const mutations = await installMutationRecorder(page)
    await installReadFixtures(page)
    const ready = route.ready(page)

    // When: 使用 DOMContentLoaded 打开目标路由并等待唯一语义锚点。
    await gotoReady(page, route.path, ready)

    // Then: 页面唯一语义标题正确，且没有浏览器、网络或写入异常。
    await expect(ready).toBeVisible()
    expect(mutations.requests).toEqual([])
    expectHealthy(health)
  })
}

const unavailableRoutes = ["/author/articles/new", "/author/articles/101/edit", "/author/taxonomy"] as const
for (const path of unavailableRoutes) {
  test(`生产作者路由 ${path} 保持不可用`, async ({ page }) => {
    // Given: 记录任何绕过生产保护的写请求。
    const health = observePageHealth(page)
    const mutations = await installMutationRecorder(page)
    await installReadFixtures(page)
    const ready = page.getByRole("heading", { name: "写作功能暂不可用", level: 1 })

    // When: 直接访问受保护作者路由。
    await gotoReady(page, path, ready)

    // Then: 呈现真实不可用边界，不重定向到登录且零 mutation。
    await expect(page).toHaveURL(path)
    await expect(ready).toBeVisible()
    expect(mutations.requests).toEqual([])
    expectHealthy(health)
  })
}

test("管理根路由与未来子路由使用独立不可用边界", async ({ page }) => {
  // Given: 两个管理地址都不需要公共读取。
  const health = observePageHealth(page)
  for (const path of ["/admin", "/admin/future-route"] as const) {
    // When: 导航到管理域地址。
    const ready = page.getByRole("heading", { name: "管理后台暂不可用", level: 1 })
    await gotoReady(page, path, ready)
    // Then: 由 AdminUnavailable 接管，而不是公共 404。
    await expect(ready).toBeVisible()
    await expect(page.getByText("404", { exact: true })).toHaveCount(0)
  }
  expectHealthy(health)
})

const legacyTruthTable: readonly { readonly from: string; readonly to: string; readonly ready: (page: Page) => Locator }[] = [
  { from: "/main", to: "/", ready: (page) => page.locator("#hero-profile-name") },
  { from: "/article/101", to: "/articles/101", ready: (page) => page.getByRole("heading", { name: "在 Edge 中验证中文博客的完整阅读体验", level: 1 }) },
  { from: "/writeBlog", to: "/author/articles/new", ready: (page) => page.getByRole("heading", { name: "写作功能暂不可用", level: 1 }) },
  { from: "/writeBlog?id=101", to: "/author/articles/101/edit", ready: (page) => page.getByRole("heading", { name: "写作功能暂不可用", level: 1 }) },
]
for (const route of legacyTruthTable) {
  test(`旧地址 ${route.from} 按真值表规范化`, async ({ page }) => {
    // Given: 旧详情重定向仍使用确定性读取 fixture。
    const health = observePageHealth(page)
    await installReadFixtures(page)
    const ready = route.ready(page)
    // When: 打开旧地址。
    await gotoReady(page, route.from, ready)
    // Then: URL 与唯一稳定标题均符合映射。
    await expect(page).toHaveURL(route.to)
    await expect(ready).toBeVisible()
    expectHealthy(health)
  })
}

for (const path of ["/article/not-a-number", "/article/0", "/writeBlog?id=broken", "/writeBlog?id=0"] as const) {
  test(`畸形旧文章标识 ${path} 留在原路由`, async ({ page }) => {
    // Given: 页面不应发起文章详情请求。
    const health = observePageHealth(page)
    await installReadFixtures(page)
    const ready = page.getByRole("heading", { name: "文章标识无效", level: 1 })
    // When: 打开畸形旧地址。
    await gotoReady(page, path, ready)
    // Then: 稳定 INVALID_ARTICLE_ID 表面出现且 URL 不被改写。
    await expect(page).toHaveURL(path)
    await expect(ready).toBeVisible()
    expectHealthy(health)
  })
}

test("未知公共路径呈现公共 404", async ({ page }) => {
  // Given: 捕获所有页面异常。
  const health = observePageHealth(page)
  const ready = page.getByRole("heading", { name: "这里没有你要找的页面", level: 1 })
  // When: 打开未知公共地址。
  await gotoReady(page, "/missing-public-page", ready)
  // Then: 公共 404 明确可见。
  await expect(ready).toBeVisible()
  await expect(page.getByText("404", { exact: true })).toBeVisible()
  expectHealthy(health)
})
