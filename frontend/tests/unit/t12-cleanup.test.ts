import { access, readFile, readdir, stat } from "node:fs/promises"
import { join } from "node:path"
import { describe, expect, it } from "vitest"

/** 旧管理演示目录与文件，拆分名称避免约束测试自身携带遗留业务文案。 */
const obsoletePaths = [
  join("src", "views", "admin", "dash" + "board"),
  join("src", "views", "admin", "lay" + "out"),
  join("src", "views", "admin", "pack" + "ages"),
  join("src", "views", "admin", "sub" + "scriptions"),
  join("src", "mock"),
  join("src", "types", "pack" + "age.ts"),
  join("src", "types", "sub" + "scription.ts"),
] as const

/** 需要检查的源码、文档与构建输入。 */
const staticRoots = ["src", "README.md", "index.html", "package.json", "vite.config.ts", "playwright.config.ts"] as const
const forbiddenTerms = [
  "SCYG Blog " + "Admin",
  "admin/" + "dashboard",
  "admin/" + "packages",
  "admin/" + "subscriptions",
  "Packages" + "View",
  "Subscriptions" + "View",
  "套" + "餐",
  "订" + "阅",
] as const

/** 递归读取可进入生产构建的文本文件，空目录贡献空文本。 */
async function collectText(path: string): Promise<string> {
  const pathStats = await stat(path)
  if (pathStats.isFile()) return readFile(path, "utf8")
  if (!pathStats.isDirectory()) return ""

  const entries = await readdir(path, { withFileTypes: true })
  if (entries.length === 0) return ""
  const contents = await Promise.all(entries.map((entry) => collectText(join(path, entry.name))))
  return contents.join("\n")
}

describe("T12 static cleanup contract", () => {
  it("keeps obsolete admin demo paths absent", async () => {
    // Given / When: 逐一探测已废弃路径。
    const results = await Promise.all(obsoletePaths.map((path) => access(path).then(() => true, () => false)))
    // Then: 所有路径均保持不存在。
    expect(results).toEqual(obsoletePaths.map(() => false))
  })

  it("keeps build inputs free of obsolete admin product language", async () => {
    // Given: 汇总生产源码、文档与构建输入，不扫描测试自身。
    const source = (await Promise.all(staticRoots.map(collectText))).join("\n")
    // When / Then: 每个旧身份、路由与业务词都不得回流。
    for (const term of forbiddenTerms) expect(source).not.toContain(term)
  })

  it("retains the admin boundary and T11 feedback primitives", async () => {
    // Given / When: 读取必须保留的路由与共享反馈实现。
    const retained = await Promise.all([
      readFile("src/router/modules/admin.ts", "utf8"),
      readFile("src/views/admin/AdminUnavailableView.vue", "utf8"),
      readFile("src/components/shared/AppModal.vue", "utf8"),
      readFile("src/components/shared/AppToast.vue", "utf8"),
    ])
    // Then: 四个保留入口均为非空源码。
    expect(retained.every((source) => source.trim().length > 0)).toBe(true)
  })
})
