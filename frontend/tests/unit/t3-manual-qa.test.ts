import { readFile, writeFile } from "node:fs/promises"
import { resolve } from "node:path"
import { describe, expect, it } from "vitest"
import { parseRuntimeConfig, RuntimeConfigError } from "@/config/runtime"
import { parseArticleDetail } from "@/request/api/article"
import { unsupportedAuthApi } from "@/request/api/auth"
import { unsupportedSearchApi } from "@/request/api/search"
import { ApiParseError, normalizeImageUrl } from "@/types/api"

const currentArticle = {
  id: 7, title: "QA article", slug: "qa-article", digest: "QA", content: "# QA", article_type_id: 2,
  tag_ids: [3], status: 2, support: 0, comment: 0, visited: 1, version: 1,
  created_at: "2026-07-11T00:00:00Z", updated_at: null,
}

const artifactPath = process.env["T3_QA_ARTIFACT"]

describe.skipIf(artifactPath === undefined)("T3 real-surface QA", () => {
  it("emits a binary artifact after exercising every named boundary", async () => {
    // Given: the deployed public file and current backend fixture.
    const configInput: unknown = JSON.parse(await readFile(resolve("public/config.json"), "utf8"))

    // When: happy and all named failure surfaces execute through production exports.
    const config = parseRuntimeConfig(configInput)
    const article = parseArticleDetail(currentArticle)
    const failures = [
      capture(() => parseRuntimeConfig({}), RuntimeConfigError),
      capture(() => parseArticleDetail({ id: "bad" }), ApiParseError),
      capture(() => normalizeImageUrl("javascript:alert(1)", config.serverUrl), ApiParseError),
      (await unsupportedAuthApi.login({ username: "qa", password: "qa" })).kind === "unsupported",
      (await unsupportedSearchApi.search({ query: "qa", pageIndex: 1, pageSize: 20 })).kind === "unsupported",
    ]
    const passed = article.markdown === "# QA" && failures.every(Boolean)

    // Then: the external artifact has one unambiguous binary outcome.
    await writeFile(artifactPath ?? "", `${JSON.stringify({ result: passed ? "PASS" : "FAIL", failures })}\n`, "utf8")
    expect(passed).toBe(true)
  })
})

/** 执行预期失败并验证稳定错误类型。 */
function capture(action: () => unknown, errorType: typeof RuntimeConfigError | typeof ApiParseError): boolean {
  try {
    action()
    return false
  } catch (error) {
    return error instanceof errorType
  }
}
