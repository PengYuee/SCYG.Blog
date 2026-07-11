import { describe, expect, it } from "vitest"
import { sanitizeMarkdown } from "@/security/sanitize-markdown"

const XSS_FIXTURES = [
  ["script", "<p>safe</p><script>alert(1)</script>", "script"],
  ["onerror", '<img src="https://img.test/safe.png" onerror="alert(1)">', "onerror"],
  ["javascript", '<a href="javascript:alert(1)">unsafe</a>', "javascript:"],
  ["iframe", '<iframe src="https://evil.test"></iframe>', "iframe"],
  ["srcdoc", '<div srcdoc="<script>alert(1)</script>">safe</div>', "srcdoc"],
  ["heading injection", '<h2 id="safe" onclick="alert(1)"><img src=x onerror=alert(1)>Title</h2>', "onclick"],
] as const

describe("sanitizeMarkdown", () => {
  it("preserves required Markdown output when content is safe", () => {
    // Given: rendered heading, code, table, image, and link output.
    const html = '<h2 id="features">Features</h2><pre><code class="language-ts">const safe = true</code></pre><table><thead><tr><th>Name</th></tr></thead><tbody><tr><td>SCYG</td></tr></tbody></table><img src="/images/safe.png" alt="safe" title="image"><a href="https://example.com/docs">Docs</a>'
    // When: the shared Markdown boundary sanitizes the output.
    const clean = sanitizeMarkdown(html)
    // Then: all required structures remain and external links are isolated.
    expect(clean).toContain('<h2 id="features">Features</h2>')
    expect(clean).toContain('<code class="language-ts">')
    expect(clean).toContain("<table>")
    expect(clean).toContain('src="/images/safe.png"')
    expect(clean).toContain('target="_blank"')
    expect(clean).toContain('rel="noopener noreferrer"')
  })

  it.each(XSS_FIXTURES)("removes %s payloads", (_name, dirty, forbidden) => {
    // Given / When: one attacker-controlled HTML fixture crosses the shared boundary.
    const clean = sanitizeMarkdown(dirty)
    // Then: its executable primitive is absent.
    expect(clean.toLowerCase()).not.toContain(forbidden)
  })

  it.each([
    ['<img src="data:image/png;base64,abc" alt="x">', "data image"],
    ['<img src="javascript:alert(1)" alt="x">', "javascript image"],
    ['<img src="//evil.test/x.png" alt="x">', "protocol-relative image"],
    ['<img src="/\\evil.test/x.png" alt="x">', "backslash-normalized image"],
    ['<a href="/\\evil.test/x">file</a>', "backslash-normalized link"],
    ['<a href="ftp://evil.test/file">file</a>', "unknown link protocol"],
  ])("removes unsafe URL from %s (%s)", (dirty) => {
    // Given / When / Then: unsafe URL schemes cannot survive sanitization.
    expect(sanitizeMarkdown(dirty)).not.toMatch(/(?:src|href)=/u)
  })
})
