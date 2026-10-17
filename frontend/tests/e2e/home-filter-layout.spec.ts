import { expect, test } from "@playwright/test"
import { installMutationRecorder, installReadFixtures } from "./t13/fixtures/api-fixtures"
import { expectHealthy, gotoReady, observePageHealth } from "./t13/helpers/page-health"

for (const theme of ["night", "sky"] as const) {
  test(`keeps three separate rails and a readable article list in ${theme}`, async ({ page }, testInfo) => {
    // Given: 两种主题使用同一套真实组件和确定性读取数据。
    const health = observePageHealth(page)
    await page.addInitScript((value) => localStorage.setItem("scyg-blog-theme", value), theme)
    await page.emulateMedia({ reducedMotion: "reduce" })
    await installReadFixtures(page)
    await gotoReady(page, "/?articleTypeId=1&tagId=1", page.getByTestId("article-grid"))

    // When: 查看内容区的三栏及已选筛选。
    await expect(page.locator(".home-author-rail")).toBeVisible()
    await expect(page.locator(".home-filter-rail")).toBeVisible()
    await expect(page.locator("#content-search-input")).toBeVisible()
    const geometry = await page.locator(".home-content-grid").evaluate((element) => {
      const rails = [...element.children].map(child => child.getBoundingClientRect())
      const articles = [...element.querySelectorAll(".article-list article")].map(child => child.getBoundingClientRect())
      return {
        widths: rails.map(rect => rect.width),
        separated: rails.every((rect, index) => index === 0 || rect.left >= (rails[index - 1]?.right ?? 0)),
        stacked: articles.every((rect, index) => index === 0 || rect.top >= (articles[index - 1]?.bottom ?? 0)),
        overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
      }
    })

    // Then: 左右栏固定，文章单列且无页面横向溢出。
    expect(geometry.widths[0]).toBe(220)
    expect(geometry.widths[2]).toBe(280)
    expect(geometry.separated).toBe(true)
    expect(geometry.stacked).toBe(true)
    expect(geometry.overflow).toBe(0)
    const coveredControls = await page.locator(".home-filter-rail").evaluate(element => {
      const switcher = document.querySelector<HTMLElement>('button[aria-label^="切换到"]')?.getBoundingClientRect()
      if (switcher === undefined) return []
      return [...element.querySelectorAll<HTMLElement>("button,input,textarea")].filter(control => {
        const rect = control.getBoundingClientRect()
        return rect.width > 0 && rect.height > 0 && rect.left < switcher.right && rect.right > switcher.left && rect.top < switcher.bottom && rect.bottom > switcher.top
      }).map(control => control.getAttribute("aria-label") ?? control.textContent)
    })
    expect(coveredControls).toEqual([])
    await expect(page.locator("#category-filter-options .filter-clear")).toBeVisible()
    await expect(page.locator("#tag-filter-options .filter-clear")).toBeVisible()
    await expect(page.locator(".article-list [role='img']")).toHaveCount(0)
    await page.screenshot({ path: testInfo.outputPath(`content-${theme}.png`) })
    await page.evaluate(() => window.scrollTo({ top: document.documentElement.scrollHeight, behavior: "instant" }))
    await page.screenshot({ path: testInfo.outputPath(`content-${theme}-end.png`) })
    expectHealthy(health)
  })
}

test("combines sidebar filters with keywords and clears only the requested condition", async ({ page }) => {
  // Given: 关键词和分类已应用，所有请求在 HTTP 边界被隔离。
  await installReadFixtures(page)
  const mutations = await installMutationRecorder(page)
  await gotoReady(page, "/?q=Edge&articleTypeId=1", page.getByTestId("article-grid"))

  // When: 叠加标签，提交新关键词，再取消标签。
  const tagRequest = page.waitForRequest(request => {
    const url = new URL(request.url())
    return url.pathname === "/api/v1/articles" && url.searchParams.get("tagId") === "2"
  })
  const tagOption = page.locator('#tag-filter-options [data-filter-id="2"]')
  await tagOption.focus()
  await tagOption.press("Enter")
  const selected = new URL((await tagRequest).url())
  expect(selected.searchParams.get("q")).toBe("Edge")
  expect(selected.searchParams.get("articleTypeId")).toBe("1")
  await expect(page).toHaveURL(/tagId=2/)
  await expect(tagOption).toBeFocused()
  await page.locator("#content-search-input").fill("Vue")
  const keywordRequest = page.waitForRequest(request => {
    const url = new URL(request.url())
    return url.pathname === "/api/v1/articles" && url.searchParams.get("q") === "Vue"
  })
  await page.locator("#content-search-input").press("Enter")
  const searched = new URL((await keywordRequest).url())
  expect(searched.searchParams.get("articleTypeId")).toBe("1")
  expect(searched.searchParams.get("tagId")).toBe("2")
  const clearRequest = page.waitForRequest(request => {
    const url = new URL(request.url())
    return url.pathname === "/api/v1/articles" && url.searchParams.get("q") === "Vue" && !url.searchParams.has("tagId")
  })
  await page.locator("#tag-filter-options .filter-clear").click()
  const cleared = new URL((await clearRequest).url())

  // Then: 取消只移除标签，关键词与分类保留，且没有外部写操作。
  expect(cleared.searchParams.get("q")).toBe("Vue")
  expect(cleared.searchParams.get("articleTypeId")).toBe("1")
  await expect(page.locator('#tag-filter-options [data-filter-id="2"]')).toBeFocused()
  await expect(page.locator("#tag-filter-options .filter-clear")).toHaveCount(0)
  expect(mutations.requests).toEqual([])
})

test("handles thirty categories with expansion and selected-first cancellation", async ({ page }) => {
  // Given: 三十个分类中包含长名称，默认只展示六项。
  await installReadFixtures(page)
  await page.route("**/api/v1/article-types?*", route => route.fulfill({ json: {
    items: Array.from({ length: 30 }, (_, index) => ({
      id: index + 1,
      name: index === 29 ? "PostgreSQL数据库迁移与持续部署实践" : `开发分类 ${index + 1}`,
      image: null,
      articleCount: 1,
    })),
    page: { number: 1, size: 100, totalItems: 30, totalPages: 1 },
  } }))
  await gotoReady(page, "/?q=Edge", page.getByTestId("article-grid"))
  const options = page.locator("#category-filter-options")
  await expect(options.locator("[data-filter-id]")).toHaveCount(6)

  // When: 展开后选择末项，再收起全部列表。
  await page.locator(".filter-card").filter({ has: options }).getByRole("button", { name: /展开全部分类/ }).click()
  await expect(options.locator("[data-filter-id]")).toHaveCount(30)
  const lastCategory = options.locator('[data-filter-id="30"]')
  await lastCategory.focus()
  await lastCategory.press("Enter")
  await expect(page).toHaveURL((url) => url.searchParams.get("q") === "Edge" && url.searchParams.get("articleTypeId") === "30")
  await expect(lastCategory).toBeFocused()
  await page.getByRole("button", { name: "收起分类", exact: true }).click()

  // Then: 选中项始终可见且置前，没有新增查找框或横向溢出。
  await expect(options.locator("[data-filter-id]")).toHaveCount(6)
  await expect(options.locator("[data-filter-id]").first()).toHaveAttribute("data-filter-id", "30")
  await expect(options.locator(".filter-clear")).toBeVisible()
  await expect(page.locator(".filter-card input")).toHaveCount(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBe(0)
})
