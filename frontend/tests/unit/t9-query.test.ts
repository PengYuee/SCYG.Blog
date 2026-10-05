import { describe, expect, it } from "vitest"
import { parseArticleListQuery, serializeArticleListQuery } from "@/router/query"

describe("T9 article discovery query boundary", () => {
  it("round trips q articleTypeId and tagId in deterministic order", () => {
    const parsed = parseArticleListQuery({ tagId: "9", q: "  Vue Router  ", articleTypeId: "2" })
    expect(parsed).toEqual({ kind: "valid", value: { q: "Vue Router", articleTypeId: 2, tagId: 9 } })
    if (parsed.kind === "valid") expect(Object.keys(serializeArticleListQuery(parsed.value))).toEqual(["q", "articleTypeId", "tagId"])
  })

  it.each([{ articleTypeId: "0" }, { articleTypeId: "1.5" }, { tagId: "oops" }, { q: ["one", "two"] }])("returns a typed failure for invalid query", (query) => {
    const result = parseArticleListQuery(query)
    expect(result).toMatchObject({ kind: "invalid", code: "INVALID_ARTICLE_QUERY" })
    expect(result).not.toHaveProperty("redirect")
  })
})
