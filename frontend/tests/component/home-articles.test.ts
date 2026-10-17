import { flushPromises, mount } from "@vue/test-utils"
import { defineComponent, h } from "vue"
import { createMemoryHistory, createRouter } from "vue-router"
import { expect, it, vi } from "vitest"
import { apiServicesKey, createApiServices } from "@/request/api-services"
import type { HttpTransport } from "@/request/transport"
import { useHomeArticles } from "@/services/home-articles"
import { scrollRestorationKey } from "@/services/scroll-restoration"
import type { PageResult } from "@/types/api"
import type { ArticleDetail } from "@/types/article"

it("delivers an in-flight cached page to a remounted homepage", async () => {
  // Given: 首页第一页完成，第二页保持在途。
  const transport: HttpTransport = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() }
  const services = createApiServices(transport, "https://blog.test")
  const item = (id: number): ArticleDetail => ({ id, title: `文章${id}`, slug: `article-${id}`, digest: "", markdown: "", articleTypeId: 1, tagIds: [], status: 2, support: 0, comment: 0, visited: 0, version: 1, createdAt: "2026-10-16T00:00:00Z", updatedAt: null })
  const deferred = Promise.withResolvers<PageResult<ArticleDetail>>()
  vi.spyOn(services.article, "list").mockResolvedValueOnce({ items: Array.from({ length: 9 }, (_, i) => item(i + 1)), pageIndex: 0, pageSize: 9, totalItems: 10, totalPages: 2 }).mockReturnValueOnce(deferred.promise)
  vi.spyOn(services.publicTaxonomy, "listArticleTypes").mockResolvedValue([])
  vi.spyOn(services.publicTaxonomy, "listTags").mockResolvedValue([])
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: "/", component: { template: "<div />" } }] })
  await router.push("/")
  const states: ReturnType<typeof useHomeArticles>[] = []
  const host = defineComponent({ setup() { const state = useHomeArticles({}); states.push(state); return () => h("div", `${state.feedState.value.kind}:${state.feedState.value.items.length}`) } })
  const options = { global: { plugins: [router], provide: { [apiServicesKey]: services, [scrollRestorationKey]: { markReady: vi.fn() } } } }
  const first = mount(host, options)
  await flushPromises()
  const original = states.at(0)
  if (original === undefined) throw new Error("Missing mounted homepage")
  const loading = original.loadFeed()
  first.unmount()
  const second = mount(host, options)
  await flushPromises()
  expect(second.text()).toBe("loading:9")
  // When: 第二页在新首页挂载后返回。
  deferred.resolve({ items: [item(10)], pageIndex: 1, pageSize: 9, totalItems: 10, totalPages: 2 })
  await loading
  await flushPromises()
  // Then: 新消费者收到完成状态与追加文章，不停留在旧 loading 快照。
  expect(second.text()).toBe("ready:10")
  second.unmount()
})
