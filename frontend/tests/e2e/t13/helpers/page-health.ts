import { expect, type Locator, type Page, type TestInfo } from "@playwright/test"
import path from "node:path"

/** 单页运行期间按来源分离的浏览器与网络异常记录。 */
export type PageHealth = {
  readonly consoleErrors: string[]
  readonly pageErrors: string[]
  readonly failedRequests: string[]
}

/** 失败场景允许的精确错误模式。 */
type ExpectedPageHealth = {
  readonly consoleErrors?: readonly RegExp[]
  readonly failedRequests?: readonly RegExp[]
}

/** 注册每页独立的控制台、页面与失败请求观察。 */
export function observePageHealth(page: Page): PageHealth {
  const consoleErrors: string[] = []
  const pageErrors: string[] = []
  const failedRequests: string[] = []
  page.on("console", (message) => { if (message.type() === "error") consoleErrors.push(message.text()) })
  page.on("pageerror", (error) => pageErrors.push(error.message))
  page.on("requestfailed", (request) => {
    if (request.failure()?.errorText !== "net::ERR_BLOCKED_BY_CLIENT") failedRequests.push(`${request.method()} ${request.url()} ${request.failure()?.errorText ?? "未知错误"}`)
  })
  return { consoleErrors, pageErrors, failedRequests }
}

/** 使用 DOMContentLoaded 与调用方提供的唯一语义 Locator 完成导航。 */
export async function gotoReady(page: Page, url: string, ready: Locator): Promise<void> {
  await page.goto(url, { waitUntil: "domcontentloaded" })
  await expect(ready).toHaveCount(1)
  await expect(ready).toBeVisible()
}

/** 等待字体与图片终态，并显式结束有限动画、取消无限动画。 */
export async function settleVisualState(page: Page): Promise<void> {
  await page.evaluate(async () => {
    await document.fonts.ready
    await Promise.all(Array.from(document.images).map((image) => {
      if (image.complete) return Promise.resolve()
      return new Promise<void>((resolve) => {
        // 先绑定 load/error，再复查 complete，避免状态切换落在检查与监听之间。
        const settled = (): void => {
          image.removeEventListener("load", settled)
          image.removeEventListener("error", settled)
          resolve()
        }
        image.addEventListener("load", settled, { once: true })
        image.addEventListener("error", settled, { once: true })
        if (image.complete) settled()
      })
    }))
    for (const animation of document.getAnimations()) {
      const iterations = animation.effect?.getTiming().iterations
      if (iterations === Infinity) animation.cancel()
      else if (animation.playState === "running" || animation.playState === "pending") animation.finish()
    }
  })
}

/** 将视觉证据写入统一且被忽略的 T13 路径。 */
export function evidencePath(testInfo: TestInfo, fileName: string): string {
  return path.resolve(testInfo.config.rootDir, "../.omo/evidence/zfy-blog-vue3-desktop-redesign/t13", testInfo.project.name, fileName)
}

/** 断言健康路径没有任何浏览器或网络异常。 */
export function expectHealthy(health: PageHealth): void {
  expect(health.consoleErrors).toEqual([])
  expect(health.pageErrors).toEqual([])
  expect(health.failedRequests).toEqual([])
}

/** 精确核对失败场景的预期信号，并继续禁止页面错误和额外异常。 */
export function expectExpectedHealth(health: PageHealth, expected: ExpectedPageHealth): void {
  const consolePatterns = expected.consoleErrors ?? []
  const failedRequestPatterns = expected.failedRequests ?? []
  expect(health.pageErrors).toEqual([])
  expect(health.consoleErrors).toHaveLength(consolePatterns.length)
  expect(health.failedRequests).toHaveLength(failedRequestPatterns.length)
  consolePatterns.forEach((pattern, index) => expect(health.consoleErrors[index]).toMatch(pattern))
  failedRequestPatterns.forEach((pattern, index) => expect(health.failedRequests[index]).toMatch(pattern))
}
