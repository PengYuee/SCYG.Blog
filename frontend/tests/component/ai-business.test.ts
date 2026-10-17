import { mount } from "@vue/test-utils"
import { describe, expect, it, vi } from "vitest"
import type { AgentRun } from "@/request/generated"
import AiInteraction from "@/components/ai/AiInteraction.vue"
import AiResult from "@/components/ai/AiResult.vue"
vi.mock("@/components/article/MarkdownRenderer.vue", () => ({ default: { props: ["markdown"], template: '<div>{{ markdown }}</div>' } }))
const interaction = { interactionId: "opaque-stage", type: "confirmation" as const, payload: { title: "草稿标题", outline: "真实大纲" } }
describe("AI business interactions", () => {
  it("confirmation explicitly approves and omits payload", async () => {
    const wrapper = mount(AiInteraction, { props: { interaction, disabled: false } })
    await wrapper.findAll("button")[0]?.trigger("click")
    expect(wrapper.emitted("submit")?.[0]).toEqual([{ interactionId: "opaque-stage", decision: "approve" }])
    expect(wrapper.text()).toContain("不代表保存或发布")
  })
  it("text input restores multiline draft and submits a JSON string", async () => {
    const wrapper = mount(AiInteraction, { props: { interaction: { ...interaction, type: "textInput", payload: { title: "补充要求", prompt: "说明读者和风格", min_length: 1, max_length: 16000 } }, disabled: false, draft: "保留的要求\n第二行" } })
    expect(wrapper.get("textarea").element.value).toBe("保留的要求\n第二行")
    await wrapper.get("textarea").setValue("  新要求\n第二行  ")
    await wrapper.get("form").trigger("submit")
    expect(wrapper.emitted("submit")?.[0]).toEqual([{ interactionId: "opaque-stage", decision: "submit", payload: "新要求\n第二行" }])
    expect(wrapper.emitted("draft")?.[0]).toEqual(["  新要求\n第二行  "])
  })
  it("selection supports real article cards and zero selection", async () => {
    const wrapper = mount(AiInteraction, { props: { interaction: { ...interaction, type: "selection", payload: { title: "选择参考", prompt: "只读取选中片段", min_selections: 0, max_selections: 2, options: [{ article_id: 1, title: "候选一", digest: "摘要一", status: "published" }, { article_id: 2, title: "候选二", digest: "摘要二", status: "draft" }] } }, disabled: false } })
    await wrapper.get("form").trigger("submit")
    expect(wrapper.emitted("submit")?.[0]).toEqual([{ interactionId: "opaque-stage", decision: "select", payload: { article_ids: [] } }])
    await wrapper.findAll('input[type="checkbox"]')[1]?.setValue(true)
    await wrapper.get("form").trigger("submit")
    expect(wrapper.emitted("submit")?.[1]).toEqual([{ interactionId: "opaque-stage", decision: "select", payload: { article_ids: [2] } }])
  })
  it("unknown pending payload is diagnostic, not a generic JSON editor", () => {
    const wrapper = mount(AiInteraction, { props: { interaction: { ...interaction, payload: null }, disabled: false } })
    expect(wrapper.text()).toContain("不会猜测提交内容")
    expect(wrapper.find("textarea").exists()).toBe(false)
  })
  it("result and failure remain independently visible", () => {
    const run: AgentRun = { runId: "run", capability: "polish", status: "failed", recipeId: "polish-v1", recipeVersion: "v1", createdAt: "2026-10-10T00:00:00Z", updatedAt: "2026-10-10T00:00:00Z", streamUrl: "/api/v1/runs/run/events", pendingInteraction: null, result: { content: "润色正文", change_summary: "缩短句子" }, failure: { code: "partial", message: "保留部分结果" } }
    const wrapper = mount(AiResult, { props: { run, anonymous: false, original: "原文" }, global: { stubs: { RouterLink: true } } })
    expect(wrapper.text()).toContain("润色正文")
    expect(wrapper.text()).toContain("原文")
    expect(wrapper.text()).toContain("保留部分结果")
    expect(wrapper.text()).toContain("缩短句子")
  })
})
