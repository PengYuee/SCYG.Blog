import { describe, expect, it } from "vitest"
import { ApiParseError, normalizeImageUrl } from "@/types/api"
import { parseArticleDetail, parseArticleList } from "@/request/api/article"
import { parseArticleTypes } from "@/request/api/article-type"
import { parseTags } from "@/request/api/tag"

const article = { id: 7, title: "Typed boundaries", slug: "typed-boundaries", digest: "Boundary parsing", content: "# Markdown", articleTypeId: 2, articleType: { id: 2, name: "Tech", image: null }, tagIds: [3], status: 2, support: 4, comment: 5, visited: 6, version: 1, createdAt: "2026-07-11T00:00:00Z", updatedAt: null }

describe("API response parsers", () => {
  it("maps a current article list envelope", () => {
    const result = parseArticleList({ items: [article], page: { number: 1, size: 20, totalItems: 1, totalPages: 1 } })
    expect(result).toMatchObject({ items: [{ id: 7, articleTypeId: 2, markdown: "# Markdown" }], pageIndex: 0, pageSize: 20, totalItems: 1 })
  })

  it("maps current article content to Markdown source", () => expect(parseArticleDetail(article).markdown).toBe("# Markdown"))

  it("rejects undeclared article fields", () => expect(() => parseArticleDetail({ ...article, unexpected: true })).toThrow(ApiParseError))

  it("rejects duplicate article tag identifiers", () => expect(() => parseArticleDetail({ ...article, tagIds: [3, 3] })).toThrow(ApiParseError))

  it("accepts documented dictionary items envelopes", () => {
    const page = { number: 1, size: 20, totalItems: 1, totalPages: 1 }
    expect(parseArticleTypes({ items: [{ id: 2, name: "Tech", image: "/media/tech.png", menu: 1, version: 1, createdAt: "2026-07-11T00:00:00Z", updatedAt: null }], page }, "https://api.test")[0]?.imageUrl).toBe("https://api.test/media/tech.png")
    expect(parseTags({ items: [{ id: 3, name: "Vue", version: 1, createdAt: "2026-07-11T00:00:00Z", updatedAt: null }], page })[0]?.name).toBe("Vue")
  })

  it.each([{ items: [{}] }, { data: "wrong" }, null])("rejects malformed article lists", (fixture) => expect(() => parseArticleList(fixture)).toThrow(ApiParseError))
  it.each(["javascript:alert(1)", "data:image/svg+xml,x", "//evil.example/image.png", "http://["])("rejects unsafe image URL %s", (value) => expect(() => normalizeImageUrl(value, "https://api.test")).toThrow(ApiParseError))
})
