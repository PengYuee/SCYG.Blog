import { describe, expect, it } from "vitest"
import { ApiParseError, normalizeImageUrl } from "@/types/api"
import { parseArticleDetail, parseArticleList } from "@/request/api/article"
import { parseArticleTypes } from "@/request/api/article-type"
import { parseTags } from "@/request/api/tag"

const article = {
  id: 7, title: "Typed boundaries", slug: "typed-boundaries", digest: "Boundary parsing", content: "# Markdown",
  article_type_id: 2, tag_ids: [3], status: 2, support: 4, comment: 5, visited: 6, version: 1,
  created_at: "2026-07-11T00:00:00Z", updated_at: null,
}

describe("API response parsers", () => {
  it("maps a current article list envelope", () => {
    // Given: the authoritative API list shape.
    const fixture: unknown = { items: [article], page: { number: 1, size: 20, total_items: 1, total_pages: 1 } }
    // When: it crosses the adapter boundary.
    const result = parseArticleList(fixture)
    // Then: snake-case fields become readonly domain fields.
    expect(result).toMatchObject({ items: [{ id: 7, articleTypeId: 2, markdown: "# Markdown" }], pageIndex: 1, pageSize: 20, totalItems: 1 })
  })

  it("maps current article content to Markdown source", () => {
    // Given / When: a current detail resource is parsed.
    const result = parseArticleDetail(article)
    // Then: backend content is the domain Markdown source.
    expect(result.markdown).toBe("# Markdown")
  })

  it("accepts documented dictionary items envelopes", () => {
    // Given: current ArticleTypeList and TagList envelopes.
    const page = { number: 1, size: 20, total_items: 1, total_pages: 1 }
    // When / Then: both dictionaries map to domain terms.
    expect(parseArticleTypes({ items: [{ id: 2, name: "Tech", image: "/media/tech.png", meun: 1, version: 1, created_at: "2026-07-11T00:00:00Z", updated_at: null }], page }, "https://api.test")[0]?.imageUrl).toBe("https://api.test/media/tech.png")
    expect(parseTags({ items: [{ id: 3, name: "Vue", version: 1, created_at: "2026-07-11T00:00:00Z", updated_at: null }], page })[0]?.name).toBe("Vue")
  })

  it.each([{ items: [{}] }, { data: "wrong" }, null])("rejects malformed article lists", (fixture) => {
    // Given / When / Then: malformed external input becomes a typed parse failure.
    expect(() => parseArticleList(fixture)).toThrow(ApiParseError)
  })

  it.each(["javascript:alert(1)", "data:image/svg+xml,x", "//evil.example/image.png", "http://["])("rejects unsafe image URL %s", (value) => {
    // Given / When / Then: unsafe or malformed locations never reach rendering.
    expect(() => normalizeImageUrl(value, "https://api.test")).toThrow(ApiParseError)
  })
})
