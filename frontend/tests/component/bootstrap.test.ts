import { describe, expect, it, vi } from "vitest"
import { bootstrapApplication, STARTUP_CONFIG_ERROR_MESSAGE } from "@/bootstrap"
import { http } from "@/request/http"

/** 创建包含应用根节点的独立启动文档。 */
function createStartupDocument(): Document {
  return document.implementation.createHTMLDocument("启动测试")
}

/** 为独立文档添加应用挂载根节点。 */
function appendApplicationRoot(targetDocument: Document): HTMLElement {
  const root = targetDocument.createElement("div")
  root.id = "app"
  targetDocument.body.append(root)
  return root
}

describe("runtime-config bootstrap", () => {
  it("configures HTTP before importing and mounting the Vue application", async () => {
    // Given: 配置响应和可观察的动态应用加载器。
    delete http.defaults.baseURL
    const targetDocument = createStartupDocument()
    appendApplicationRoot(targetDocument)
    const mountApplication = vi.fn()
    const loadApplication = vi.fn(async () => {
      expect(http.defaults.baseURL).toBe("http://localhost:5000/api")
      return { mountApplication }
    })

    // When: 启动边界完成配置加载。
    await bootstrapApplication({
      document: targetDocument,
      fetcher: async () => new Response('{"serverUrl":"http://localhost:5000/api"}'),
      loadApplication,
    })

    // Then: 业务模块只在 HTTP 初始化后导入并收到同一配置。
    expect(loadApplication).toHaveBeenCalledOnce()
    expect(mountApplication).toHaveBeenCalledWith({ serverUrl: "http://localhost:5000/api" })
  })

  it("shows a Chinese alert and makes zero business requests when config fails", async () => {
    // Given: config.json 返回失败且业务模块可被观察。
    delete http.defaults.baseURL
    const targetDocument = createStartupDocument()
    const root = appendApplicationRoot(targetDocument)
    const loadApplication = vi.fn()

    // When: 启动配置请求失败。
    await bootstrapApplication({
      document: targetDocument,
      fetcher: async () => new Response("missing", { status: 404 }),
      loadApplication,
    })

    // Then: Vue 不导入、不挂载，页面显示稳定中文告警且无业务基址。
    expect(loadApplication).not.toHaveBeenCalled()
    expect(http.defaults.baseURL).toBeUndefined()
    expect(root.querySelector('[role="alert"]')?.textContent).toBe(STARTUP_CONFIG_ERROR_MESSAGE)
  })
})