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

/** 检测关键中文文本的实际裁切，同时允许标题自然多行展开。 */
export async function expectCjkUnclipped(locator: Locator): Promise<void> {
  const measurements = await locator.evaluateAll((elements) => elements.map((element) => {
    if (!(element instanceof HTMLElement)) return { tag: "UNKNOWN", overflowX: "", overflowY: "", lineClamp: "", visible: false, widthFits: false, strictHeightFits: false, verticalSafe: false, insideViewport: false, singleLineControl: false }
    const rectangle = element.getBoundingClientRect()
    const style = getComputedStyle(element)
    const tag = element.tagName
    const singleLineControl = tag === "BUTTON" || tag === "INPUT" || tag === "SELECT"
    const strictHeightFits = element.scrollHeight <= element.clientHeight + 1
    const clipsVerticalOverflow = style.overflowY === "hidden" || style.overflowY === "clip"
    const lineClamp = style.webkitLineClamp
    const hasLineClamp = lineClamp !== "none" && lineClamp !== "" && lineClamp !== "0"
    return {
      tag,
      overflowX: style.overflowX,
      overflowY: style.overflowY,
      lineClamp,
      visible: element.clientWidth > 0 && element.clientHeight > 0,
      widthFits: element.scrollWidth <= element.clientWidth + 1,
      strictHeightFits,
      // 自然多行标题可高于单行内容盒；只有隐藏溢出或 line-clamp 时才要求严格高度拟合。
      verticalSafe: strictHeightFits || (!singleLineControl && !clipsVerticalOverflow && !hasLineClamp),
      insideViewport: rectangle.left >= -1 && rectangle.right <= document.documentElement.clientWidth + 1,
      singleLineControl,
    }
  }))
  expect(measurements.length).toBeGreaterThan(0)
  for (const measurement of measurements) {
    expect(measurement.visible, `${measurement.tag} 应具有非零可见内容盒`).toBe(true)
    expect(measurement.widthFits, `${measurement.tag} 不应发生水平裁切`).toBe(true)
    expect(measurement.insideViewport, `${measurement.tag} 必须保持在视口内`).toBe(true)
    expect(measurement.verticalSafe, `${measurement.tag} 不应被 overflow 或 line-clamp 垂直截断`).toBe(true)
    if (measurement.singleLineControl) expect(measurement.strictHeightFits, `${measurement.tag} 单行控件必须完整容纳内容`).toBe(true)
  }
}
