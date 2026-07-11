import { expect, test } from "@playwright/test"
import { writeFile } from "node:fs/promises"
import { installReadFixtures } from "./fixtures/api-fixtures"
import { evidencePath, gotoReady, settleVisualState } from "./helpers/page-health"

/** 运行时参考证据状态，不可达时必须显式记录而非伪造通过。 */
type ReferenceEvidence = {
  readonly status: "captured" | "skipped-unconfigured" | "skipped-unreachable"
  readonly viewport: { readonly width: number; readonly height: number }
  readonly implementationPath?: string
  readonly referencePath?: string
  readonly reason?: string
}

const referenceUrl = process.env["T13_REFERENCE_URL"]

test("生成同视口的新鲜实现与外部参考证据", async ({ browser, page }, testInfo) => {
  // Given: 实现页面使用确定性 fixture，并固定为当前精确 project 视口。
  const viewport = page.viewportSize()
  expect(viewport).not.toBeNull()
  await page.emulateMedia({ reducedMotion: "reduce" })
  await installReadFixtures(page)
  await gotoReady(page, "/", "[data-testid='article-grid']")
  await settleVisualState(page)
  const implementationPath = evidencePath(testInfo, "reference-implementation.png")
  await page.screenshot({ path: implementationPath, fullPage: true, animations: "disabled" })

  // When: 未配置外部参考时，写入可审计 skip 证据，不制造参考截图。
  const manifestPath = evidencePath(testInfo, "reference-evidence.json")
  if (referenceUrl === undefined || referenceUrl.length === 0) {
    const evidence: ReferenceEvidence = { status: "skipped-unconfigured", viewport: viewport ?? { width: 0, height: 0 }, implementationPath, reason: "未配置 T13_REFERENCE_URL" }
    await writeFile(manifestPath, `${JSON.stringify(evidence, null, 2)}\n`, "utf8")
    test.skip(true, "外部参考未配置；已记录显式 skip 证据")
    return
  }

  // When: 参考可达时使用同一 Edge/Chromium browser 与完全相同视口现场捕获。
  const context = await browser.newContext({ viewport: viewport ?? { width: 1280, height: 720 }, reducedMotion: "reduce" })
  const referencePage = await context.newPage()
  const response = await referencePage.goto(referenceUrl, { waitUntil: "domcontentloaded", timeout: 15_000 }).catch(() => null)
  if (response === null || !response.ok()) {
    const evidence: ReferenceEvidence = { status: "skipped-unreachable", viewport: viewport ?? { width: 0, height: 0 }, implementationPath, reason: response === null ? "参考地址不可达" : `参考响应 ${response.status()}` }
    await writeFile(manifestPath, `${JSON.stringify(evidence, null, 2)}\n`, "utf8")
    await context.close()
    test.skip(true, evidence.reason)
    return
  }

  // Then: 仅保存现场参考像素与路径清单，不复制品牌资源、名称或文案到仓库。
  await referencePage.evaluate(async () => { await document.fonts.ready; await Promise.all(document.getAnimations().map((animation) => animation.finished)) })
  const referencePath = evidencePath(testInfo, "reference-external.png")
  await referencePage.screenshot({ path: referencePath, fullPage: true, animations: "disabled" })
  const evidence: ReferenceEvidence = { status: "captured", viewport: viewport ?? { width: 0, height: 0 }, implementationPath, referencePath }
  await writeFile(manifestPath, `${JSON.stringify(evidence, null, 2)}\n`, "utf8")
  await context.close()
})
