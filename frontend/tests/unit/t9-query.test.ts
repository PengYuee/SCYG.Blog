import { describe, expect, it } from "vitest"
import { parseArticleListQuery, serializeArticleListQuery } from "@/router/query"

describe("T9 article discovery query boundary", () => {
  it("round trips q categoryId and tagId in deterministic order", () => {
    // Given: 外部查询参数包含空白和合法正整数。
    const input = { tagId: "9", q: "  Vue Router  ", categoryId: "2" }
    // When: 查询被解析后重新序列化。
    const parsed = parseArticleListQuery(input)
    // Then: 值被规范化，键顺序固定为 q/categoryId/tagId。
    expect(parsed).toEqual({ kind: "valid", value: { q: "Vue Router", categoryId: 2, tagId: 9 } })
    if (parsed.kind === "valid") expect(Object.keys(serializeArticleListQuery(parsed.value))).toEqual(["q", "categoryId", "tagId"])
  })

  it.each([
    { categoryId: "0" },
    { categoryId: "1.5" },
    { tagId: "oops" },
    { q: ["one", "two"] },
  ])("returns a typed failure for invalid query $categoryId$tagId", (query) => {
    // Given / When: URL 查询含非法或重复字段。
    const result = parseArticleListQuery(query)
    // Then: 边界显式失败，不生成重定向目标。
    expect(result).toMatchObject({ kind: "invalid", code: "INVALID_ARTICLE_QUERY" })
    expect(result).not.toHaveProperty("redirect")
  })
})
