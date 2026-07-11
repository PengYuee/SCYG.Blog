import { expect, type Page, type TestInfo } from "@playwright/test"
import path from "node:path"

/** 单页运行期间的浏览器与网络异常记录。 */
export type PageHealth = {
  readonly errors: string[]
  readonly failedRequests: string[]
}

/** 注册每页独立的控制台、页面与失败请求观察。 */
export function observePageHealth(page: Page): PageHealth {
  const errors: string[] = []
  const failedRequests: string[] = []
  page.on("console", (message) => { if (message.type() === "error") errors.push(`console:${message.text()}`) })
  page.on("pageerror", (error) => errors.push(`page:${error.message}`))
  page.on("requestfailed", (request) => {
    if (request.failure()?.errorText !== "net::ERR_BLOCKED_BY_CLIENT") failedRequests.push(`${request.method()} ${request.url()}`)
  })
  return { errors, failedRequests }
}

/** 使用 DOMContentLoaded 与页面显式就绪锚点完成导航。 */
export async function gotoReady(page: Page, url: string, readySelector: string): Promise<void> {
  await page.goto(url, { waitUntil: "domcontentloaded" })
  await expect(page.locator(readySelector)).toBeVisible()
}

/** 等待字体、图片与 Web Animations 进入稳定完成态。 */
export async function settleVisualState(page: Page): Promise<void> {
  await page.evaluate(async () => {
    await document.fonts.ready
    await Promise.all(Array.from(document.images).map(async (image) => {
      if (image.complete) return
      await new Promise<void>((resolve) => image.addEventListener("load", () => resolve(), { once: true }))
    }))
    await Promise.all(document.getAnimations().map((animation) => animation.finished))
  })
}

/** 将视觉证据写入统一且被忽略的 T13 路径。 */
export function evidencePath(testInfo: TestInfo, fileName: string): string {
  return path.resolve(testInfo.config.rootDir, "../.omo/evidence/zfy-blog-vue3-desktop-redesign/t13", testInfo.project.name, fileName)
}

/** 断言页面没有未预期的浏览器或网络异常。 */
export function expectHealthy(health: PageHealth): void {
  expect(health.errors).toEqual([])
  expect(health.failedRequests).toEqual([])
}
