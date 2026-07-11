import { expect, type Locator, type Page } from "@playwright/test"

/** 首页桌面几何快照。 */
type DesktopGeometry = {
  readonly heroHeight: number
  readonly contentTop: number
  readonly viewportHeight: number
  readonly shellWidth: number
  readonly sidebarWidth: number
  readonly columns: number
  readonly overflow: number
}

/** 验证 T6 锁定的 Hero、内容边界、壳层、侧栏、网格与溢出契约。 */
export async function expectDesktopGeometry(page: Page): Promise<void> {
  const geometry = await page.evaluate<DesktopGeometry | null>(() => {
    const hero = document.querySelector<HTMLElement>(".public-hero")
    const content = document.querySelector<HTMLElement>("#blog-content")
    const shell = content?.querySelector<HTMLElement>(".blog-shell")
    const sidebar = document.querySelector<HTMLElement>(".blog-sidebar")
    const grid = document.querySelector<HTMLElement>("[data-testid='article-grid']")
    if (hero === null || content === null || shell === undefined || sidebar === null || grid === null) return null
    return {
      heroHeight: hero.getBoundingClientRect().height,
      contentTop: content.getBoundingClientRect().top + window.scrollY,
      viewportHeight: window.innerHeight,
      shellWidth: shell.getBoundingClientRect().width,
      sidebarWidth: sidebar.getBoundingClientRect().width,
      columns: getComputedStyle(grid).gridTemplateColumns.split(" ").length,
      overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    }
  })
  expect(geometry).not.toBeNull()
  expect(Math.abs((geometry?.heroHeight ?? 0) - (geometry?.viewportHeight ?? 1))).toBeLessThanOrEqual(1)
  expect(Math.abs((geometry?.contentTop ?? 0) - (geometry?.heroHeight ?? 1))).toBeLessThanOrEqual(1)
  expect(geometry?.shellWidth).toBeLessThanOrEqual(1220)
  expect(geometry?.sidebarWidth).toBe(300)
  expect(geometry?.columns).toBe(3)
  expect(geometry?.overflow).toBe(0)
}

/** 检测关键中文文本的裁切、滚动溢出与越界。 */
export async function expectCjkUnclipped(locator: Locator): Promise<void> {
  const measurements = await locator.evaluateAll((elements) => elements.map((element) => {
    if (!(element instanceof HTMLElement)) return { visible: false, widthFits: false, heightFits: false, insideViewport: false }
    const rectangle = element.getBoundingClientRect()
    return {
      visible: rectangle.width > 0 && rectangle.height > 0,
      widthFits: element.scrollWidth <= Math.ceil(rectangle.width),
      heightFits: element.scrollHeight <= Math.ceil(rectangle.height),
      insideViewport: rectangle.left >= 0 && rectangle.right <= document.documentElement.clientWidth,
    }
  }))
  expect(measurements.length).toBeGreaterThan(0)
  for (const measurement of measurements) expect(measurement).toEqual({ visible: true, widthFits: true, heightFits: true, insideViewport: true })
}