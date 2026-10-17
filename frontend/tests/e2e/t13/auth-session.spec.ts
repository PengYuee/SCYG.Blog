import { expect, test, type Page } from "@playwright/test"
import type { LoginResponse, Problem } from "../../../src/request/generated"
import { installMutationRecorder, installReadFixtures } from "./fixtures/api-fixtures"
import { expectExpectedHealth, expectHealthy, gotoReady, observePageHealth, waitForHeroBackground } from "./helpers/page-health"

const sessionKey = "scyg.auth.session"
const loginPath = "/api/v1/auth/login"
const categoryPath = "/api/v1/manage/article-types"
const tagPath = "/api/v1/manage/tags"
const testUsername = "e2e-author"
const testPassword = "e2e-password"

function session(expiresAt = new Date(Date.now() + 3_600_000).toISOString()): LoginResponse {
  return { accessToken: "opaque-e2e-access-token", tokenType: "Bearer", expiresAt }
}

async function storedSession(page: Page): Promise<string | null> {
  return page.evaluate((key) => localStorage.getItem(key), sessionKey)
}

async function submitLogin(page: Page): Promise<void> {
  await page.getByLabel("用户名", { exact: true }).fill(testUsername)
  await page.getByLabel("密码", { exact: true }).fill(testPassword)
  await page.getByRole("button", { name: "登录", exact: true }).click()
}

async function installLogin(page: Page, response: LoginResponse): Promise<void> {
  // 未显式配置的写请求一律拦截，测试不会触及真实后端数据。
  await installMutationRecorder(page)
  await installReadFixtures(page)
  await page.route(`**${loginPath}`, async (route) => {
    if (route.request().method() !== "POST") {
      await route.fallback()
      return
    }
    await route.fulfill({ status: 200, json: response })
  })
}

test("登录失败保留深链、显示后端错误并清空密码", async ({ page }) => {
  const health = observePageHealth(page)
  await installMutationRecorder(page)
  await installReadFixtures(page)
  const problem = { type: "/problems/unauthorized", title: "未认证", status: 401, detail: "用户名或密码错误", instance: loginPath, requestId: "e2e-auth-request", errors: {} } satisfies Problem
  await page.route(`**${loginPath}`, async (route) => {
    if (route.request().method() !== "POST") {
      await route.fallback()
      return
    }
    await route.fulfill({ status: 401, contentType: "application/problem+json", json: problem })
  })
  await gotoReady(page, "/author/taxonomy", page.getByRole("heading", { name: "作者登录", level: 1 }))

  const requestPromise = page.waitForRequest((request) => new URL(request.url()).pathname === loginPath && request.method() === "POST")
  await submitLogin(page)
  const request = await requestPromise

  expect(request.postDataJSON()).toEqual({ username: testUsername, password: testPassword })
  expect(request.headers()["authorization"]).toBeUndefined()
  await expect(page.getByRole("alert")).toHaveText("用户名或密码错误")
  await expect(page.getByLabel("密码", { exact: true })).toHaveValue("")
  await expect(page.getByLabel("用户名", { exact: true })).toHaveValue(testUsername)
  await expect(page).toHaveURL((url) => url.pathname === "/login" && url.searchParams.get("redirect") === "/author/taxonomy")
  expect(await storedSession(page)).toBeNull()
  expectExpectedHealth(health, { consoleErrors: [/^Failed to load resource: the server responded with a status of 401 \(Unauthorized\)$/] })
})

test("深链登录携带 Bearer，刷新恢复会话，退出后重新要求登录", async ({ page }) => {
  const health = observePageHealth(page)
  const response = session()
  await installLogin(page, response)
  await gotoReady(page, "/author/taxonomy", page.getByRole("heading", { name: "作者登录", level: 1 }))

  const categoryRequest = page.waitForRequest((request) => new URL(request.url()).pathname === categoryPath && request.method() === "GET")
  const tagRequest = page.waitForRequest((request) => new URL(request.url()).pathname === tagPath && request.method() === "GET")
  await submitLogin(page)
  for (const request of await Promise.all([categoryRequest, tagRequest])) {
    expect(request.headers()["authorization"]).toBe(`Bearer ${response.accessToken}`)
  }
  await expect(page).toHaveURL("/author/taxonomy")
  await expect(page.getByRole("heading", { name: "文章分类", level: 2 })).toBeVisible()
  await expect(page.getByText("Vue 3", { exact: true })).toBeVisible()
  expect(JSON.parse((await storedSession(page)) ?? "null")).toEqual(response)

  const restoredRequest = page.waitForRequest((request) => new URL(request.url()).pathname === categoryPath && request.method() === "GET")
  await page.reload({ waitUntil: "domcontentloaded" })
  expect((await restoredRequest).headers()["authorization"]).toBe(`Bearer ${response.accessToken}`)
  await expect(page.getByRole("heading", { name: "文章分类", level: 2 })).toBeVisible()
  await page.getByRole("button", { name: "退出登录", exact: true }).click()
  await expect(page).toHaveURL("/")
  await expect(page.locator("#hero-profile-name")).toBeVisible()
  await waitForHeroBackground(page)
  expect(await storedSession(page)).toBeNull()

  await gotoReady(page, "/author/articles/new", page.getByRole("heading", { name: "作者登录", level: 1 }))
  await expect(page).toHaveURL((url) => url.pathname === "/login" && url.searchParams.get("redirect") === "/author/articles/new")
  expectHealthy(health)
})

test("没有返回地址时登录进入可用的新建文章页面", async ({ page }) => {
  const health = observePageHealth(page)
  await installLogin(page, session())
  await gotoReady(page, "/login", page.getByRole("heading", { name: "作者登录", level: 1 }))

  await submitLogin(page)

  await expect(page).toHaveURL("/author/articles/new")
  await expect(page.getByRole("heading", { name: "新建文章", level: 1 })).toBeVisible()
  await expect(page.getByTestId("save-article")).toBeEnabled()
  expectHealthy(health)
})

test("已过期的持久会话被清除且不读取作者数据", async ({ page }) => {
  const health = observePageHealth(page)
  const requests: string[] = []
  await installMutationRecorder(page)
  await installReadFixtures(page)
  page.on("request", (request) => {
    if (new URL(request.url()).pathname.startsWith("/api/v1/manage/") && request.method() === "GET") requests.push(request.url())
  })
  await page.addInitScript(({ key, response }) => localStorage.setItem(key, JSON.stringify(response)), { key: sessionKey, response: session(new Date(Date.now() - 60_000).toISOString()) })

  await gotoReady(page, "/author/articles/101/edit", page.getByRole("heading", { name: "作者登录", level: 1 }))

  await expect(page.getByRole("alert")).toHaveText("登录会话已过期，请重新登录")
  await expect(page).toHaveURL((url) => url.pathname === "/login" && url.searchParams.get("redirect") === "/author/articles/101/edit")
  expect(await storedSession(page)).toBeNull()
  expect(requests).toEqual([])
  expectHealthy(health)
})

test("浏览器内会话过期后作者导航重新要求登录", async ({ page }) => {
  const health = observePageHealth(page)
  const now = Date.now()
  await page.clock.setFixedTime(new Date(now))
  await installLogin(page, session(new Date(now + 60_000).toISOString()))
  await gotoReady(page, "/login", page.getByRole("heading", { name: "作者登录", level: 1 }))
  await submitLogin(page)
  await expect(page.getByTestId("save-article")).toBeEnabled()

  await page.clock.setFixedTime(new Date(now + 120_000))
  await page.getByRole("link", { name: "分类与标签", exact: true }).click()

  await expect(page).toHaveURL((url) => url.pathname === "/login" && url.searchParams.get("redirect") === "/author/taxonomy")
  expect(await storedSession(page)).toBeNull()
  expectHealthy(health)
})

test("受保护请求返回 401 后清除会话并在下一次作者导航要求登录", async ({ page }) => {
  const health = observePageHealth(page)
  const response = session()
  await installLogin(page, response)
  await gotoReady(page, "/author/taxonomy", page.getByRole("heading", { name: "作者登录", level: 1 }))
  await submitLogin(page)
  await expect(page.getByRole("heading", { name: "文章分类", level: 2 })).toBeVisible()
  const problem = { type: "/problems/unauthorized", title: "未认证", status: 401, detail: "登录令牌已失效", instance: categoryPath, requestId: "e2e-auth-request", errors: {} } satisfies Problem
  await page.route(`**${categoryPath}?*`, async (route) => {
    if (route.request().method() !== "GET") {
      await route.fallback()
      return
    }
    await route.fulfill({ status: 401, contentType: "application/problem+json", json: problem })
  })

  const rejectedRequest = page.waitForRequest((request) => new URL(request.url()).pathname === categoryPath && request.method() === "GET")
  await page.getByRole("link", { name: "新文章", exact: true }).click()
  expect((await rejectedRequest).headers()["authorization"]).toBe(`Bearer ${response.accessToken}`)
  await expect(page.getByRole("heading", { name: "写作台加载失败", level: 2 })).toBeVisible()
  expect(await storedSession(page)).toBeNull()
  await page.getByRole("link", { name: "分类与标签", exact: true }).click()

  await expect(page).toHaveURL((url) => url.pathname === "/login" && url.searchParams.get("redirect") === "/author/taxonomy")
  await expect(page.getByLabel("用户名", { exact: true })).toBeVisible()
  expect(await storedSession(page)).toBeNull()
  expectExpectedHealth(health, { consoleErrors: [/^Failed to load resource: the server responded with a status of 401 \(Unauthorized\)$/] })
})
