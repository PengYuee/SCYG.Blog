import { expect, test } from "@playwright/test"
import { writeFile } from "node:fs/promises"
import { installReadFixtures } from "./fixtures/api-fixtures"
import { evidencePath, gotoReady, settleVisualState } from "./helpers/page-health"

/** 运行时参考证据状态，不可达、未渲染或空白时不得标记 captured。 */
type ReferenceEvidence = {
  readonly status: "captured" | "skipped-unconfigured" | "skipped-unreachable" | "failed-render" | "rejected-blank"
  readonly referenceUrl: string | null
  readonly viewport: { readonly width: number; readonly height: number }
  readonly implementationPath?: string
  readonly referencePath?: string
  readonly reason?: string
}

const referenceUrl = process.env["T13_REFERENCE_URL"]
const referenceRenderTimeoutMs = 20_000

test("生成同视口的新鲜实现与外部参考证据", async ({ browser, page }, testInfo) => {
  // Given: 实现页面使用确定性 fixture，并固定为当前精确 project 视口。
  const viewport = page.viewportSize()
  expect(viewport).not.toBeNull()
  await page.emulateMedia({ reducedMotion: "reduce" })
  await installReadFixtures(page)
  const latestGrid = page.getByRole("heading", { name: "最新文章", level: 2 }).locator("xpath=ancestor::section").getByTestId("article-grid")
  await gotoReady(page, "/", latestGrid)
  await settleVisualState(page)
  const implementationPath = evidencePath(testInfo, "reference-implementation.png")
  await page.screenshot({ path: implementationPath, fullPage: true, animations: "disabled" })

  // When: 未配置外部参考时，写入可审计 skip 证据，不制造参考截图。
  const manifestPath = evidencePath(testInfo, "reference-evidence.json")
  if (referenceUrl === undefined || referenceUrl.length === 0) {
    const evidence: ReferenceEvidence = { status: "skipped-unconfigured", referenceUrl: null, viewport: viewport ?? { width: 0, height: 0 }, implementationPath, reason: "未配置 T13_REFERENCE_URL" }
    await writeFile(manifestPath, `${JSON.stringify(evidence, null, 2)}\n`, "utf8")
    test.skip(true, "外部参考未配置；已记录显式 skip 证据")
    return
  }

  // When: 参考可达时使用同一浏览器与完全相同视口现场捕获。
  const context = await browser.newContext({ viewport: viewport ?? { width: 1280, height: 720 }, reducedMotion: "reduce" })
  const referencePage = await context.newPage()
  const response = await referencePage.goto(referenceUrl, { waitUntil: "domcontentloaded", timeout: 15_000 }).catch(() => null)
  if (response === null || !response.ok()) {
    const evidence: ReferenceEvidence = { status: "skipped-unreachable", referenceUrl, viewport: viewport ?? { width: 0, height: 0 }, implementationPath, reason: response === null ? "参考地址不可达" : `参考响应 ${response.status()}` }
    await writeFile(manifestPath, `${JSON.stringify(evidence, null, 2)}\n`, "utf8")
    await context.close()
    test.skip(true, evidence.reason)
    return
  }

  // Then: 最多等待 20 秒，直到 SPA 根节点拥有真实可见内容而非空壳。
  try {
    await referencePage.waitForFunction(() => {
      const app = document.querySelector<HTMLElement>("#app")
      if (app === null) return false
      const visibleDescendants = Array.from(app.querySelectorAll<HTMLElement>("*")).filter((element) => {
        const rectangle = element.getBoundingClientRect()
        const style = getComputedStyle(element)
        return rectangle.width > 0 && rectangle.height > 0 && style.display !== "none" && style.visibility !== "hidden" && style.opacity !== "0"
      })
      const textReady = (app.textContent ?? "").trim().length > 0
      const geometryReady = visibleDescendants.some((element) => {
        const rectangle = element.getBoundingClientRect()
        const style = getComputedStyle(element)
        return rectangle.width * rectangle.height >= 400 && (element.matches("img,svg,canvas,video") || style.backgroundImage !== "none")
      })
      return visibleDescendants.length > 0 && (textReady || geometryReady)
    }, undefined, { timeout: referenceRenderTimeoutMs })
  } catch (error) {
    if (!(error instanceof Error)) throw error
    const evidence: ReferenceEvidence = { status: "failed-render", referenceUrl, viewport: viewport ?? { width: 0, height: 0 }, implementationPath, reason: error.message }
    await writeFile(manifestPath, `${JSON.stringify(evidence, null, 2)}\n`, "utf8")
    await context.close()
    throw error
  }

  await settleVisualState(referencePage, referenceRenderTimeoutMs)
  const rendered = await referencePage.evaluate(() => {
    const app = document.querySelector<HTMLElement>("#app")
    if (app === null) return { visibleDescendants: 0, appArea: 0, viewportArea: window.innerWidth * window.innerHeight, textLength: 0, mediaCount: 0, coloredSurfaces: 0, effectiveBackground: "missing", effectivelyBlank: true }
    const descendants = Array.from(app.querySelectorAll<HTMLElement>("*")).filter((element) => {
      const rectangle = element.getBoundingClientRect()
      const style = getComputedStyle(element)
      return rectangle.width > 0 && rectangle.height > 0 && style.display !== "none" && style.visibility !== "hidden" && style.opacity !== "0"
    })
    const appRectangle = app.getBoundingClientRect()
    const bodyBackground = getComputedStyle(document.body).backgroundColor
    const htmlBackground = getComputedStyle(document.documentElement).backgroundColor
    const effectiveBackground = bodyBackground === "rgba(0, 0, 0, 0)" ? htmlBackground : bodyBackground
    const rgb = effectiveBackground.match(/\d+(?:\.\d+)?/g)?.slice(0, 3).map(Number) ?? []
    const nearWhite = rgb.length === 3 && rgb.every((channel) => channel >= 245)
    const mediaCount = descendants.filter((element) => element.matches("img,svg,canvas,video") && element.getBoundingClientRect().width * element.getBoundingClientRect().height >= 400).length
    const coloredSurfaces = descendants.filter((element) => {
      const style = getComputedStyle(element)
      return style.backgroundImage !== "none" || (!style.backgroundColor.includes("0, 0, 0, 0") && style.backgroundColor !== "transparent" && !style.backgroundColor.includes("255, 255, 255"))
    }).length
    const textLength = (app.textContent ?? "").replace(/\s/g, "").length
    const appArea = appRectangle.width * appRectangle.height
    const viewportArea = window.innerWidth * window.innerHeight
    const effectivelyBlank = descendants.length === 0 || appArea < viewportArea * 0.1 || (textLength === 0 && mediaCount === 0) || (nearWhite && coloredSurfaces === 0 && mediaCount === 0 && textLength < 20)
    return { visibleDescendants: descendants.length, appArea, viewportArea, textLength, mediaCount, coloredSurfaces, effectiveBackground, effectivelyBlank }
  })

  if (rendered.effectivelyBlank) {
    const evidence: ReferenceEvidence = { status: "rejected-blank", referenceUrl, viewport: viewport ?? { width: 0, height: 0 }, implementationPath, reason: `参考页面空白: ${JSON.stringify(rendered)}` }
    await writeFile(manifestPath, `${JSON.stringify(evidence, null, 2)}\n`, "utf8")
    await context.close()
    expect(rendered.effectivelyBlank, evidence.reason).toBe(false)
    return
  }

  // Then: 仅保存真实渲染后的参考像素与路径清单，不复制参考内容到仓库。
  const referencePath = evidencePath(testInfo, "reference-external.png")
  await referencePage.screenshot({ path: referencePath, fullPage: true, animations: "disabled" })
  const evidence: ReferenceEvidence = { status: "captured", referenceUrl, viewport: viewport ?? { width: 0, height: 0 }, implementationPath, referencePath }
  await writeFile(manifestPath, `${JSON.stringify(evidence, null, 2)}\n`, "utf8")
  await context.close()
})