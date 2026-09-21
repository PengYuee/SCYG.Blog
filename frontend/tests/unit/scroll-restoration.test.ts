// @vitest-environment jsdom
import { describe, expect, it, vi } from "vitest"
import { createSafeStorage, createScrollRestoration } from "@/services/scroll-restoration"

class TestImage extends EventTarget {
  /** 模拟浏览器图片完成状态。 */
  complete = false
  /** 返回当前图片监听器数量。 */
  listenerCount = 0
  override addEventListener(type: string, callback: EventListenerOrEventListenerObject | null, options?: AddEventListenerOptions | boolean): void {
    this.listenerCount += 1
    super.addEventListener(type, callback, options)
  }
  override removeEventListener(type: string, callback: EventListenerOrEventListenerObject | null, options?: EventListenerOptions | boolean): void {
    this.listenerCount -= 1
    super.removeEventListener(type, callback, options)
  }
}

/** 创建可精确推进动画帧与超时的浏览器端口。 */
function createHarness(options: { readonly navigationType?: string; readonly stored?: string; readonly fullPath?: string; readonly images?: readonly TestImage[] } = {}) {
  const listeners = new Map<string, Set<EventListener>>()
  const storage = new Map<string, string>()
  if (options.stored !== undefined) storage.set("scyg-blog:scroll-restoration", options.stored)
  let scrollHeight = 2400
  let scrollY = 0
  let fullPath = options.fullPath ?? "/articles"
  let resizeListener: (() => void) | undefined
  const timerHandles = new Set<number>()
  const frameHandles = new Set<number>()
  const scrollTo = vi.fn((position: ScrollToOptions) => { scrollY = position.top ?? scrollY })
  const port = {
    navigationType: options.navigationType ?? "reload",
    get fullPath() { return fullPath },
    get scrollY() { return scrollY },
    get innerHeight() { return 900 },
    get scrollHeight() { return scrollHeight },
    setScrollY(value: number) { scrollY = value },
    setScrollHeight(value: number) { scrollHeight = value },
    setFullPath(value: string) { fullPath = value },
    storage: {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => { storage.set(key, value) },
      removeItem: (key: string) => { storage.delete(key) },
    },
    addEventListener(type: string, listener: EventListener, eventOptions?: AddEventListenerOptions) {
      const registered = listeners.get(type) ?? new Set<EventListener>()
      if (eventOptions?.once === true) {
        const onceListener: EventListener = (event) => {
          registered.delete(onceListener)
          listener(event)
        }
        registered.add(onceListener)
      } else {
        registered.add(listener)
      }
      listeners.set(type, registered)
    },
    removeEventListener(type: string, listener: EventListener) { listeners.get(type)?.delete(listener) },
    requestAnimationFrame(callback: FrameRequestCallback) {
      const handle = window.setTimeout(() => { frameHandles.delete(handle); callback(0) }, 0)
      frameHandles.add(handle)
      return handle
    },
    cancelAnimationFrame(handle: number) { frameHandles.delete(handle); window.clearTimeout(handle) },
    setTimeout(callback: () => void, delay: number) {
      const handle = window.setTimeout(() => { timerHandles.delete(handle); callback() }, delay)
      timerHandles.add(handle)
      return handle
    },
    clearTimeout(handle: number) { timerHandles.delete(handle); window.clearTimeout(handle) },
    nextTick: async () => undefined,
    images: () => options.images ?? [],
    observeDocumentResize(callback: () => void) { resizeListener = callback; return () => { resizeListener = undefined } },
    scrollTo,
    restore: (top: number) => scrollTo({ top }),
    dispatch(type: string, event = new Event(type)) { for (const listener of listeners.get(type) ?? []) listener(event) },
    listenerCount(type: string) { return listeners.get(type)?.size ?? 0 },
    pendingFrameCount() { return frameHandles.size },
    pendingTimerCount() { return timerHandles.size },
    resizeObserverCount() { return resizeListener === undefined ? 0 : 1 },
    notifyResize() { resizeListener?.() },
    storedValue() { return storage.get("scyg-blog:scroll-restoration") },
  }
  return port
}

const positionRecord = JSON.stringify({ version: 1, fullPath: "/articles", target: { kind: "position", top: 640 } })
const bottomRecord = JSON.stringify({ version: 1, fullPath: "/articles", target: { kind: "bottom" } })

describe("reload scroll restoration", () => {
  it("restores a matching reload position once after readiness and stable frames", async () => {
    vi.useFakeTimers()
    const port = createHarness({ stored: positionRecord })
    const service = createScrollRestoration(port)
    service.install()
    expect(service.arm("/articles")).toBe(true)
    service.markReady()
    const result = service.wait()
    await vi.runAllTimersAsync()
    expect(await result).toBe(false)
    expect(port.scrollTo).toHaveBeenCalledWith({ top: 640 })
    expect(service.arm("/articles")).toBe(false)
    expect(port.storedValue()).toBeUndefined()
    vi.useRealTimers()
  })

  it("waits for ready before calculating the final bottom", async () => {
    vi.useFakeTimers()
    const port = createHarness({ stored: bottomRecord })
    const service = createScrollRestoration(port)
    service.install()
    service.markReady()
    expect(service.arm("/articles")).toBe(true)
    port.setScrollHeight(3600)
    const result = service.wait()
    await vi.runAllTimersAsync()
    expect(await result).toBe(false)
    expect(port.scrollTo).toHaveBeenCalledWith({ top: 2700 })
    vi.useRealTimers()
  })

  it.each([
    ["navigate", positionRecord, "/articles"],
    ["reload", positionRecord, "/other"],
    ["reload", "not-json", "/articles"],
    ["reload", JSON.stringify({ version: 1, fullPath: "", target: { kind: "position", top: 10 } }), ""],
  ])("does not arm for navigation=%s or mismatched/malformed state", (navigationType, stored, fullPath) => {
    const service = createScrollRestoration(createHarness({ navigationType, stored }))
    service.install()
    expect(service.arm(fullPath)).toBe(false)
  })

  it("times out after five seconds and removes temporary listeners", async () => {
    vi.useFakeTimers()
    const port = createHarness({ stored: positionRecord })
    const service = createScrollRestoration(port)
    service.install()
    service.arm("/articles")
    const result = service.wait()
    await vi.advanceTimersByTimeAsync(5000)
    expect(await result).toBe(false)
    expect(port.listenerCount("wheel")).toBe(0)
    expect(port.pendingFrameCount()).toBe(0)
    expect(port.pendingTimerCount()).toBe(0)
    expect(port.resizeObserverCount()).toBe(0)
    vi.useRealTimers()
  })

  it("cancels on user intent and captures bottom on pagehide", async () => {
    const port = createHarness({ stored: positionRecord })
    const service = createScrollRestoration(port)
    service.install()
    service.arm("/articles")
    const result = service.wait()
    port.dispatch("wheel")
    expect(await result).toBe(false)
    port.setScrollY(1500)
    port.dispatch("pagehide")
    expect(port.storedValue()).toContain('"kind":"bottom"')
  })

  it("does not restore after a programmatic navigation changes the exact full path", async () => {
    vi.useFakeTimers()
    const port = createHarness({ stored: positionRecord })
    const service = createScrollRestoration(port)
    service.install()
    service.arm("/articles")
    const result = service.wait()
    port.setFullPath("/articles/42")
    service.markReady()
    await vi.runAllTimersAsync()
    expect(await result).toBe(false)
    expect(port.scrollTo).not.toHaveBeenCalled()
    expect(port.pendingFrameCount()).toBe(0)
    expect(port.pendingTimerCount()).toBe(0)
    expect(port.resizeObserverCount()).toBe(0)
    vi.useRealTimers()
  })

  it("cancel permanently finishes pending work even after returning to the same path", async () => {
    vi.useFakeTimers()
    const port = createHarness({ stored: positionRecord })
    const service = createScrollRestoration(port)
    service.install()
    service.arm("/articles")
    const result = service.wait()
    service.cancel()
    port.setFullPath("/other")
    port.setFullPath("/articles")
    service.markReady()
    await vi.runAllTimersAsync()
    expect(await result).toBe(false)
    expect(port.scrollTo).not.toHaveBeenCalled()
    expect(port.pendingFrameCount()).toBe(0)
    expect(port.pendingTimerCount()).toBe(0)
    vi.useRealTimers()
  })

  it("cancel during image waiting removes image listeners and every temporary resource", async () => {
    vi.useFakeTimers()
    const image = new TestImage()
    const port = createHarness({ stored: bottomRecord, images: [image] })
    const service = createScrollRestoration(port)
    service.install()
    service.arm("/articles")
    const result = service.wait()
    service.markReady()
    await Promise.resolve()
    expect(image.listenerCount).toBe(2)
    service.cancel()
    expect(await result).toBe(false)
    expect(image.listenerCount).toBe(0)
    expect(port.pendingFrameCount()).toBe(0)
    expect(port.pendingTimerCount()).toBe(0)
    expect(port.resizeObserverCount()).toBe(0)
    vi.useRealTimers()
  })

  it("captures every pagehide in the same document and overwrites with the latest route", () => {
    const port = createHarness()
    const service = createScrollRestoration(port)
    service.install()
    port.setScrollY(300)
    port.dispatch("pagehide")
    expect(port.storedValue()).toContain('"top":300')
    port.setFullPath("/articles/42")
    port.setScrollY(800)
    port.dispatch("pagehide")
    expect(port.storedValue()).toContain('"fullPath":"/articles/42"')
    expect(port.storedValue()).toContain('"top":800')
  })

  it.each([positionRecord, bottomRecord])("preserves the original target when pagehide repeats before restoration completes", (stored) => {
    const port = createHarness({ stored })
    const service = createScrollRestoration(port)
    service.install()
    service.arm("/articles")
    port.setScrollHeight(900)
    port.setScrollY(0)
    port.dispatch("pagehide")
    expect(port.storedValue()).toBe(stored)
  })

  it("waits for pending images and a 200ms resize quiet window before restoring final bottom", async () => {
    vi.useFakeTimers()
    const image = new TestImage()
    const port = createHarness({ stored: bottomRecord, images: [image] })
    const service = createScrollRestoration(port)
    service.install()
    service.arm("/articles")
    const result = service.wait()
    service.markReady()
    await vi.advanceTimersByTimeAsync(300)
    expect(port.scrollTo).not.toHaveBeenCalled()
    port.setScrollHeight(3600)
    image.complete = true
    image.dispatchEvent(new Event("load"))
    await vi.advanceTimersByTimeAsync(150)
    port.setScrollHeight(4200)
    port.notifyResize()
    await vi.advanceTimersByTimeAsync(199)
    expect(port.scrollTo).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(await result).toBe(false)
    expect(port.scrollTo).toHaveBeenCalledWith({ top: 3300 })
    expect(image.listenerCount).toBe(0)
    expect(port.pendingTimerCount()).toBe(0)
    expect(port.resizeObserverCount()).toBe(0)
    vi.useRealTimers()
  })

  it("degrades storage access failures including non-Error throws to a no-op storage", () => {
    const storage = createSafeStorage(() => { throw "storage disabled" })
    expect(storage.getItem("key")).toBeNull()
    expect(() => storage.setItem("key", "value")).not.toThrow()
    expect(() => storage.removeItem("key")).not.toThrow()
  })
})
