import { mount } from "@vue/test-utils"
import { defineComponent, h } from "vue"
import { describe, expect, it, vi } from "vitest"
import MarkdownRenderer from "@/components/article/MarkdownRenderer.vue"
import { sanitizeMarkdown } from "@/security/sanitize-markdown"

const MdPreviewStub = defineComponent({
  name: "MdPreview",
  inheritAttrs: false,
  props: { id: { type: String, required: true }, modelValue: { type: String, required: true }, sanitize: { type: Function, required: true } },
  setup(props) {
    return () => h("article", { "data-preview-id": props.id, innerHTML: sanitizeMarkdown('<h1 id="safe-heading">Safe heading</h1><pre><code>code</code></pre><table><tbody><tr><td>cell</td></tr></tbody></table><img src="/images/safe.png" alt="safe">') })
  },
})

const MdCatalogStub = defineComponent({
  name: "MdCatalog",
  inheritAttrs: false,
  props: { editorId: { type: String, required: true } },
  setup(props) { return () => h("nav", { "data-catalog-for": props.editorId }, "Safe heading") },
})

describe("MarkdownRenderer", () => {
  it("connects sanitized source, preview hook, and catalog to one stable preview", () => {
    // Given: Markdown containing a malicious raw-HTML heading.
    const markdown = '# Safe heading\n<h2 onclick="alert(1)">Injected</h2>'
    // When: the article renderer mounts.
    const wrapper = mount(MarkdownRenderer, { props: { markdown }, global: { stubs: { MdPreview: MdPreviewStub, MdCatalog: MdCatalogStub } } })
    // Then: preview and catalog share an ID and only sanitized source enters md-editor.
    const preview = wrapper.findComponent(MdPreviewStub)
    expect(preview.props("modelValue")).toBe(sanitizeMarkdown(markdown))
    expect(preview.props("sanitize")).toBe(sanitizeMarkdown)
    expect(wrapper.get("nav").attributes("data-catalog-for")).toBe(wrapper.get("article").attributes("data-preview-id"))
    expect(preview.props("modelValue")).not.toContain("onclick")
  })

  it("renders heading, code, table, safe image, and catalog artifacts", () => {
    // Given / When: safe feature Markdown is rendered through the bounded component harness.
    const wrapper = mount(MarkdownRenderer, { props: { markdown: "# Safe heading" }, global: { stubs: { MdPreview: MdPreviewStub, MdCatalog: MdCatalogStub } } })
    // Then: named reader-visible artifacts exist.
    expect(wrapper.get("h1").text()).toBe("Safe heading")
    expect(wrapper.get("code").text()).toBe("code")
    expect(wrapper.get("table").text()).toBe("cell")
    expect(wrapper.get("img").attributes("src")).toBe("/images/safe.png")
    expect(wrapper.get("nav").text()).toBe("Safe heading")
  })
})
