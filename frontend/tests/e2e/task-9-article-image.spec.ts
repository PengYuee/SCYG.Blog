import { Buffer } from "node:buffer"
import { expect, test, type Page } from "@playwright/test"

/** 受控本地 API 根地址，与运行时配置一致。 */
const API_ROOT = "http://127.0.0.1:8080"
/** 正文图片稳定测试标识。 */
const IMAGE_ID = "0123456789abcdef0123456789abcdef"
/** 正文图片规范远程地址。 */
const IMAGE_PATH = `/media/article-images/${IMAGE_ID}.png`
/** 可被 Edge 解码的 1×1 PNG 媒体字节。 */
const PNG_BYTES = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=", "base64")
/** 可被 Edge 解码的 1×1 JPEG 媒体字节。 */
const JPEG_BYTES = Buffer.from("/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAP//////////////////////////////////////////////////////////////////////////////////////2wBDAf//////////////////////////////////////////////////////////////////////////////////////wAARCAABAAEDASIAAhEBAxEB/8QAFQABAQAAAAAAAAAAAAAAAAAAAAX/xAAUEAEAAAAAAAAAAAAAAAAAAAAA/9oADAMBAAIQAxAAAAEf/8QAFBABAAAAAAAAAAAAAAAAAAAAAP/aAAgBAQABBQJ//8QAFBEBAAAAAAAAAAAAAAAAAAAAAP/aAAgBAwEBPwF//8QAFBEBAAAAAAAAAAAAAAAAAAAAAP/aAAgBAgEBPwF//8QAFBABAAAAAAAAAAAAAAAAAAAAAP/aAAgBAQAGPwJ//8QAFBABAAAAAAAAAAAAAAAAAAAAAP/aAAgBAQABPyF//9oADAMBAAIAAwAAABD/xAAUEQEAAAAAAAAAAAAAAAAAAAAA/9oACAEDAQE/EH//xAAUEQEAAAAAAAAAAAAAAAAAAAAA/9oACAECAQE/EH//xAAUEAEAAAAAAAAAAAAAAAAAAAAA/9oACAEBAAE/EH//2Q==", "base64")

/** Edge 验收场景开关与网络观察结果。 */
type ApiScenario = {
  readonly uploadFails?: boolean
  readonly saveFails?: boolean
  readonly deleteFails?: boolean
  readonly requests: { uploads: number; deletes: string[]; articleBodies: unknown[]; multipartHeaders: string[] }
}

/** 为写作台安装可观测的受控本地 API。 */
async function installApi(page: Page, scenario: ApiScenario): Promise<void> {
  await page.route(`${API_ROOT}/**`, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    if (url.pathname === "/api/v1/article-types") {
      await route.fulfill({ json: { items: [{ id: 1, name: "工程笔记", image: null, meun: 1, version: 1, created_at: "2026-07-13T00:00:00Z", updated_at: null }], page: { number: 1, size: 100, total_items: 1, total_pages: 1 } } })
      return
    }
    if (url.pathname === "/api/v1/tags") {
      await route.fulfill({ json: { items: [{ id: 1, name: "Vue", version: 1, created_at: "2026-07-13T00:00:00Z", updated_at: null }], page: { number: 1, size: 100, total_items: 1, total_pages: 1 } } })
      return
    }
    if (url.pathname === "/api/v1/article-images" && request.method() === "POST") {
      scenario.requests.uploads += 1
      scenario.requests.multipartHeaders.push(request.headers()["content-type"] ?? "")
      if (scenario.uploadFails) {
        await route.fulfill({ status: 500, json: { detail: "受控图片上传失败" } })
        return
      }
      await route.fulfill({ status: 201, json: { id: IMAGE_ID, storageKey: `${IMAGE_ID}.png`, url: IMAGE_PATH, mediaType: "png", byteSize: 4, width: 1, height: 1, status: "pending", expiresAt: "2026-07-14T00:00:00Z" } })
      return
    }
    if (url.pathname === "/Article/CreateArticle" && request.method() === "POST") {
      scenario.requests.articleBodies.push(request.postDataJSON())
      await route.fulfill(scenario.saveFails ? { status: 500, json: { detail: "受控文章保存失败" } } : { status: 200, json: true })
      return
    }
    if (url.pathname === `/api/v1/article-images/${IMAGE_ID}` && request.method() === "DELETE") {
      scenario.requests.deletes.push(url.pathname)
      await route.fulfill(scenario.deleteFails ? { status: 500, json: { detail: "由服务端 TTL 兜底清理" } } : { status: 204 })
      return
    }
    if (url.pathname.startsWith("/media/article-images/") && request.method() === "GET") {
      if (url.pathname.endsWith(".png")) {
        await route.fulfill({ status: 200, contentType: "image/png", body: PNG_BYTES })
        return
      }
      if (url.pathname.endsWith(".jpg")) {
        await route.fulfill({ status: 200, contentType: "image/jpeg", body: JPEG_BYTES })
        return
      }
    }
    await route.fulfill({ status: 404, json: { detail: "未配置的受控接口" } })
  })
}

/** 创建独立可变观察器，变更是测试记录的唯一职责。 */
function scenario(options: Omit<ApiScenario, "requests"> = {}): ApiScenario {
  return { ...options, requests: { uploads: 0, deletes: [], articleBodies: [], multipartHeaders: [] } }
}

test("Edge 上传 JPEG 后写入远程 URL，保存成功不删除", async ({ page }) => {
  // Given: 上传与文章保存均成功的受控 API。
  const api = scenario()
  const browserErrors: string[] = []
  page.on("console", (message) => { if (message.type() === "error") browserErrors.push(`console: ${message.text()}`) })
  page.on("pageerror", (error) => { browserErrors.push(`pageerror: ${error.message}`) })
  page.on("requestfailed", (request) => { browserErrors.push(`requestfailed: ${request.method()} ${request.url()}`) })
  await installApi(page, api)
  await page.goto("/author/articles/new")
  const editor = page.getByRole("textbox").last()
  const original = await editor.textContent()

  // When: 通过真实文件输入选择 JPEG 并保存文章。
  await page.locator('input[type="file"]').first().setInputFiles("public/images/hero-starry.jpg")
  const uploadedImage = page.locator(`img[src="${API_ROOT}${IMAGE_PATH}"]`)
  await expect.poll(() => uploadedImage.evaluate((image) => image instanceof HTMLImageElement && image.complete && image.naturalWidth > 0)).toBe(true)
  await page.getByTestId("save-article").click()
  await expect(page.getByText("文章已保存").first()).toBeVisible()
  await page.screenshot({ path: "../.omo/evidence/task-9-edge-upload-save.png", fullPage: true })

  // Then: 浏览器生成 multipart boundary，Markdown 合同不增加图片字段。
  expect(original).not.toContain(IMAGE_PATH)
  expect(api.requests.multipartHeaders[0]).toMatch(/^multipart\/form-data; boundary=/)
  expect(api.requests.articleBodies[0]).toMatchObject({ body: expect.stringContaining(`${API_ROOT}${IMAGE_PATH}`) })
  expect(api.requests.articleBodies[0]).not.toHaveProperty("images")
  expect(api.requests.deletes).toEqual([])
  expect(browserErrors).toEqual([])
})

test("Edge 上传失败保持正文不变", async ({ page }) => {
  // Given: 图片上传返回受控失败。
  const api = scenario({ uploadFails: true })
  await installApi(page, api)
  await page.goto("/author/articles/new")
  const editor = page.getByRole("textbox").last()
  const original = await editor.textContent()
  // When: 选择 PNG 文件。
  await page.locator('input[type="file"]').first().setInputFiles("public/images/hero-sky.png")
  // Then: 失败反馈为中文且受控正文未变化。
  await expect(page.getByText("图片上传失败").first()).toBeVisible()
  await expect(editor).toHaveText(original)
  await page.screenshot({ path: "../.omo/evidence/task-9-edge-upload-failure.png", fullPage: true })
})

test("Edge 保存失败与离页均按 id 取消，DELETE 失败由 TTL 兜底", async ({ page }) => {
  // Given: 上传成功、文章保存失败且取消接口失败的受控 API。
  const api = scenario({ saveFails: true, deleteFails: true })
  await installApi(page, api)
  await page.goto("/author/articles/new")
  await page.locator('input[type="file"]').first().setInputFiles("public/images/hero-sky.png")
  await expect(page.locator(`img[src="${API_ROOT}${IMAGE_PATH}"]`)).toBeAttached()
  await page.getByTestId("save-article").click()
  // When: 保存失败触发取消，随后离开页面验证取消集合已收敛。
  await expect(page.getByText("保存失败").first()).toBeVisible()
  await expect(page.getByText("临时图片将由服务端过期清理").first()).toBeVisible()
  await expect.poll(() => api.requests.deletes.length).toBe(1)
  await page.getByRole("link", { name: "SCYG 写作台" }).click()
  // Then: 仅按稳定 id 删除一次，失败项留给服务端 TTL，不发生重复取消。
  expect(api.requests.deletes).toEqual([`/api/v1/article-images/${IMAGE_ID}`])
  await page.screenshot({ path: "../.omo/evidence/task-9-edge-save-failure-cleanup.png", fullPage: true })
})

test("Edge 未保存离页按 id 取消待提交图片", async ({ page }) => {
  // Given: 图片已上传但文章尚未保存。
  const api = scenario()
  await installApi(page, api)
  await page.goto("/author/articles/new")
  await page.locator('input[type="file"]').first().setInputFiles("public/images/hero-starry.jpg")
  await expect(page.locator(`img[src="${API_ROOT}${IMAGE_PATH}"]`)).toBeAttached()
  // When: 用户直接离开写作台。
  await page.getByRole("link", { name: "SCYG 写作台" }).click()
  // Then: 卸载生命周期按响应 id 发出一次取消。
  await expect.poll(() => api.requests.deletes).toEqual([`/api/v1/article-images/${IMAGE_ID}`])
})
