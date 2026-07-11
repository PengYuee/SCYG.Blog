import { readFile, writeFile } from "node:fs/promises"
import { resolve } from "node:path"
import { describe, expect, it } from "vitest"
import { loadRuntimeConfig, parseRuntimeConfig, RuntimeConfigError } from "@/config/runtime"
import { parseArticleDetail } from "@/request/api/article"
import { unsupportedAuthApi } from "@/request/api/auth"
import { unsupportedSearchApi } from "@/request/api/search"
import { ApiParseError, normalizeImageUrl } from "@/types/api"

/** 单项手动 QA 结果。 */
type QaEntry = { readonly expected: string; readonly actual: string; readonly passed: boolean }

const currentArticle = {
  id: 7, title: "QA article", slug: "qa-article", digest: "QA", content: "# QA", article_type_id: 2,
  tag_ids: [3], status: 2, support: 0, comment: 0, visited: 1, version: 1,
  created_at: "2026-07-11T00:00:00Z", updated_at: null,
}
const artifactPath = process.env["T3_QA_ARTIFACT"]

describe.skipIf(artifactPath === undefined)("T3 named real-surface QA", () => {
  it("emits expected and actual values for every required boundary", async () => {
    // Given: the deployed public file and current backend fixture.
    const configInput: unknown = JSON.parse(await readFile(resolve("public/config.json"), "utf8"))
    const config = parseRuntimeConfig(configInput)
    const unsupported = '{"kind":"unsupported","feature":"auth"}'

    // When: every named happy and failure boundary executes through production exports.
    const entries: Readonly<Record<string, QaEntry>> = {
      realConfig: valueEntry(config.serverUrl, "http://localhost:8080"),
      articleFixture: valueEntry(parseArticleDetail(currentArticle).markdown, "# QA"),
      malformedJson: await configErrorEntry(() => loadRuntimeConfig(async () => new Response("{")), "CONFIG_INVALID"),
      nonObjectJson: await configErrorEntry(() => loadRuntimeConfig(async () => new Response("[]")), "CONFIG_INVALID"),
      missingServerUrl: configErrorEntrySync(() => parseRuntimeConfig({}), "CONFIG_INVALID"),
      nonHttpUrl: configErrorEntrySync(() => parseRuntimeConfig({ serverUrl: "ftp://api.test" }), "CONFIG_INVALID"),
      fetchRejected: await configErrorEntry(() => loadRuntimeConfig(async () => Promise.reject(new TypeError("offline"))), "CONFIG_FETCH_FAILED"),
      non2xx: await configErrorEntry(() => loadRuntimeConfig(async () => new Response("missing", { status: 404 })), "CONFIG_FETCH_FAILED"),
      safeRelativeImage: valueEntry(normalizeImageUrl("/images/a.png", config.serverUrl), "http://localhost:8080/images/a.png"),
      safeHttpsImage: valueEntry(normalizeImageUrl("https://cdn.test/a.png", config.serverUrl), "https://cdn.test/a.png"),
      javascriptImage: apiErrorEntry(() => normalizeImageUrl("javascript:alert(1)", config.serverUrl)),
      dataImage: apiErrorEntry(() => normalizeImageUrl("data:image/png,x", config.serverUrl)),
      protocolRelativeImage: apiErrorEntry(() => normalizeImageUrl("//evil.test/a.png", config.serverUrl)),
      malformedImage: apiErrorEntry(() => normalizeImageUrl("http://[", config.serverUrl)),
      authLoginUnsupported: valueEntry(JSON.stringify(await unsupportedAuthApi.login({ username: "qa", password: "qa" })), unsupported),
      authRefreshUnsupported: valueEntry(JSON.stringify(await unsupportedAuthApi.refresh({ refreshToken: "opaque" })), unsupported),
      authMeUnsupported: valueEntry(JSON.stringify(await unsupportedAuthApi.me({})), unsupported),
      authLogoutUnsupported: valueEntry(JSON.stringify(await unsupportedAuthApi.logout({ refreshToken: "opaque" })), unsupported),
      searchUnsupported: valueEntry(JSON.stringify(await unsupportedSearchApi.search({ q: "qa", pageIndex: 1, pageSize: 20 })), '{"kind":"unsupported","feature":"search"}'),
    }
    const passed = Object.values(entries).every((entry) => entry.passed)

    // Then: the artifact is explicit and has one aggregate binary outcome.
    await writeFile(artifactPath ?? "", `${JSON.stringify({ result: passed ? "PASS" : "FAIL", entries }, null, 2)}\n`, "utf8")
    expect(passed).toBe(true)
  })
})

/** 比较普通值。 */
function valueEntry(actual: string, expected: string): QaEntry { return { expected, actual, passed: actual === expected } }

/** 捕获同步配置错误码。 */
function configErrorEntrySync(action: () => unknown, expected: RuntimeConfigError["code"]): QaEntry {
  try { action(); return valueEntry("NO_ERROR", expected) } catch (error) { if (error instanceof RuntimeConfigError) return valueEntry(error.code, expected); throw error }
}

/** 捕获异步配置错误码。 */
async function configErrorEntry(action: () => Promise<unknown>, expected: RuntimeConfigError["code"]): Promise<QaEntry> {
  try { await action(); return valueEntry("NO_ERROR", expected) } catch (error) { if (error instanceof RuntimeConfigError) return valueEntry(error.code, expected); throw error }
}

/** 捕获图片边界解析错误。 */
function apiErrorEntry(action: () => unknown): QaEntry {
  try { action(); return valueEntry("NO_ERROR", "API_PARSE_ERROR") } catch (error) { if (error instanceof ApiParseError) return valueEntry(error.code, "API_PARSE_ERROR"); throw error }
}
