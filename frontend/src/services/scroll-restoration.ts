import { inject, nextTick, type InjectionKey } from "vue"
import { z } from "zod"

const STORAGE_KEY = "scyg-blog:scroll-restoration"
const RECORD_VERSION = 1
const BOTTOM_THRESHOLD_PX = 2
const RESTORE_TIMEOUT_MS = 5_000
const VISUAL_QUIET_MS = 200
const CANCEL_KEYS = new Set(["ArrowDown", "ArrowUp", "End", "Home", "PageDown", "PageUp", " "])
const CANCEL_EVENTS = ["wheel", "touchstart", "pointerdown", "keydown"] as const

const restorationRecordSchema = z.object({
  version: z.literal(RECORD_VERSION),
  fullPath: z.string().min(1),
  target: z.discriminatedUnion("kind", [
    z.object({ kind: z.literal("position"), top: z.number().finite().nonnegative() }),
    z.object({ kind: z.literal("bottom") }),
  ]),
})

type RestorationRecord = z.infer<typeof restorationRecordSchema>
type RestorationTarget = RestorationRecord["target"]
type RestorationResult = false

/** 滚动恢复所需的浏览器能力端口，便于精确验证时序与清理。 */
export interface ScrollRestorationPort {
  /** 当前导航类型。 */ readonly navigationType: string | undefined
  /** 当前地址的完整路由部分。 */ readonly fullPath: string
  /** 当前垂直滚动位置。 */ readonly scrollY: number
  /** 当前视口高度。 */ readonly innerHeight: number
  /** 当前文档滚动高度。 */ readonly scrollHeight: number
  /** 会话级存储。 */ readonly storage: Pick<Storage, "getItem" | "removeItem" | "setItem">
  /** 注册短生命周期浏览器事件。 */ addEventListener(type: string, listener: EventListener, options?: AddEventListenerOptions): void
  /** 移除短生命周期浏览器事件。 */ removeEventListener(type: string, listener: EventListener, options?: EventListenerOptions): void
  /** 创建恢复超时。 */ setTimeout(callback: () => void, delay: number): number
  /** 清理恢复超时。 */ clearTimeout(handle: number): void
  /** 等待 Vue 完成当前渲染。 */ nextTick(): Promise<void>
  /** 返回当前文档中可能改变高度的图片。 */ images(): readonly ScrollRestorationImage[]
  /** 观察文档尺寸变化并返回清理函数。 */ observeDocumentResize(callback: () => void): () => void
  /** 不使用平滑动画恢复到最终像素。 */ restore(top: number): void
}

/** 核心服务等待图片所需的最小接口。 */
export interface ScrollRestorationImage {
  /** 图片是否已经完成加载或失败。 */ readonly complete: boolean
  /** 监听图片终态。 */ addEventListener(type: "load" | "error", listener: EventListener): void
  /** 清理图片终态监听。 */ removeEventListener(type: "load" | "error", listener: EventListener): void
}

/** 页面与路由共享的一次性刷新滚动恢复服务。 */
export interface ScrollRestorationService {
  /** 安装文档生命周期 pagehide 捕获并读取一次刷新候选。 */ install(): void
  /** 仅为精确匹配的初始刷新路由进入等待态。 */ arm(fullPath: string): boolean
  /** 永久取消当前刷新恢复意图。 */ cancel(): void
  /** 标记公共页面首次数据已进入终态。 */ markReady(): void
  /** 等待 ready、稳定高度或取消，并在服务内部完成一次性恢复。 */ wait(): Promise<RestorationResult>
}

type RestorationState =
  | { readonly kind: "idle" }
  | { readonly kind: "armed"; readonly fullPath: string; readonly target: RestorationTarget }
  | { readonly kind: "completed" }

type ActiveSession = {
  /** ready 后启动视觉稳定等待。 */ readonly begin: () => void
  /** 统一完成并清理全部临时资源。 */ readonly finish: () => void
}

const unavailableStorage: Pick<Storage, "getItem" | "removeItem" | "setItem"> = {
  getItem: () => null,
  removeItem: () => undefined,
  setItem: () => undefined,
}

/** 将可能被安全策略禁用的 sessionStorage 降级为无操作存储。 */
export function createSafeStorage(getStorage: () => Storage): Pick<Storage, "getItem" | "removeItem" | "setItem"> {
  let storage: Storage
  try {
    storage = getStorage()
  } catch {
    return unavailableStorage
  }
  return {
    getItem(key) {
      try { return storage.getItem(key) } catch { return null }
    },
    removeItem(key) {
      try { storage.removeItem(key) } catch { return }
    },
    setItem(key, value) {
      try { storage.setItem(key, value) } catch { return }
    },
  }
}

/** 缺失应用注入时给出稳定中文错误。 */
export class ScrollRestorationProviderError extends Error {
  constructor() {
    super("缺少滚动恢复服务提供者")
    this.name = "ScrollRestorationProviderError"
  }
}

/** Vue 组件树中的滚动恢复服务键。 */
export const scrollRestorationKey: InjectionKey<ScrollRestorationService> = Symbol("scroll-restoration")

/** 读取应用组合根提供的滚动恢复服务。 */
export function useScrollRestoration(): ScrollRestorationService {
  const service = inject(scrollRestorationKey)
  if (service === undefined) throw new ScrollRestorationProviderError()
  return service
}

/** 创建严格一次性的 reload-only 滚动恢复服务。 */
export function createScrollRestoration(port: ScrollRestorationPort): ScrollRestorationService {
  let state: RestorationState = { kind: "idle" }
  let candidate: RestorationRecord | undefined
  let ready = false
  let installed = false
  let activeSession: ActiveSession | undefined

  /** 从不可信会话存储读取并立即消费候选。 */
  function consumeCandidate(): RestorationRecord | undefined {
    try {
      const serialized = port.storage.getItem(STORAGE_KEY)
      port.storage.removeItem(STORAGE_KEY)
      if (serialized === null) return undefined
      const parsed: unknown = JSON.parse(serialized)
      const result = restorationRecordSchema.safeParse(parsed)
      return result.success ? result.data : undefined
    } catch { return undefined }
  }

  /** 在页面离开时保存当前位置或底部语义目标。 */
  function capture(): void {
    const record = state.kind === "armed"
      ? { version: RECORD_VERSION, fullPath: state.fullPath, target: state.target }
      : {
          version: RECORD_VERSION,
          fullPath: port.fullPath,
          target: Math.max(0, port.scrollHeight - port.innerHeight) - port.scrollY <= BOTTOM_THRESHOLD_PX
            ? { kind: "bottom" } as const
            : { kind: "position", top: port.scrollY } as const,
        }
    try {
      port.storage.setItem(STORAGE_KEY, JSON.stringify(record))
    } catch { return }
  }

  return {
    install() {
      if (installed) return
      installed = true
      candidate = consumeCandidate()
      port.addEventListener("pagehide", capture)
    },
    arm(fullPath) {
      if (state.kind !== "idle" || port.navigationType !== "reload" || candidate?.fullPath !== fullPath) return false
      state = { kind: "armed", fullPath, target: candidate.target }
      candidate = undefined
      return true
    },
    cancel() {
      if (state.kind !== "armed") return
      activeSession?.finish()
      if (state.kind === "armed") state = { kind: "completed" }
    },
    markReady() {
      ready = true
      activeSession?.begin()
    },
    async wait() {
      if (state.kind !== "armed") return false
      const { fullPath, target } = state
      return new Promise<RestorationResult>((resolve) => {
        const cleanups = new Set<() => void>()
        let settled = false
        let begun = false
        let quietTimer: number | undefined
        const timeoutHandle = port.setTimeout(() => finish(), RESTORE_TIMEOUT_MS)
        const cancel = (event: Event): void => {
          if (event instanceof KeyboardEvent && !CANCEL_KEYS.has(event.key)) return
          finish()
        }
        const cleanup = (): void => {
          port.clearTimeout(timeoutHandle)
          if (quietTimer !== undefined) port.clearTimeout(quietTimer)
          for (const eventName of CANCEL_EVENTS) port.removeEventListener(eventName, cancel)
          for (const dispose of cleanups) dispose()
          cleanups.clear()
        }
        const finish = (): void => {
          if (settled) return
          settled = true
          state = { kind: "completed" }
          activeSession = undefined
          cleanup()
          resolve(false)
        }
        for (const eventName of CANCEL_EVENTS) port.addEventListener(eventName, cancel, { passive: true })

        const startQuietWindow = (): void => {
          if (settled) return
          const resetQuietWindow = (): void => {
            if (quietTimer !== undefined) port.clearTimeout(quietTimer)
            quietTimer = port.setTimeout(() => {
              if (settled) return
              if (port.fullPath !== fullPath) {
                finish()
                return
              }
              port.restore(target.kind === "bottom" ? Math.max(0, port.scrollHeight - port.innerHeight) : target.top)
              finish()
            }, VISUAL_QUIET_MS)
          }
          cleanups.add(port.observeDocumentResize(resetQuietWindow))
          resetQuietWindow()
        }

        const waitForImages = (): void => {
          const pendingImages = port.images().filter((image) => !image.complete)
          if (pendingImages.length === 0) {
            startQuietWindow()
            return
          }
          let remaining = pendingImages.length
          for (const image of pendingImages) {
            const dispose = (): void => {
              image.removeEventListener("load", onComplete)
              image.removeEventListener("error", onComplete)
            }
            const onComplete = (): void => {
              dispose()
              cleanups.delete(dispose)
              remaining -= 1
              if (remaining === 0) startQuietWindow()
            }
            image.addEventListener("load", onComplete)
            image.addEventListener("error", onComplete)
            cleanups.add(dispose)
          }
        }

        const begin = (): void => {
          if (begun || settled) return
          begun = true
          void port.nextTick().then(() => {
            if (settled) return
            if (port.fullPath !== fullPath) {
              finish()
              return
            }
            waitForImages()
          })
        }
        activeSession = { begin, finish }
        if (ready) begin()
      })
    },
  }
}

/** 创建生产浏览器端口。 */
export function createBrowserScrollRestoration(): ScrollRestorationService {
  const navigationEntry = performance.getEntriesByType("navigation")[0]
  const navigationType = navigationEntry !== undefined && "type" in navigationEntry && typeof navigationEntry.type === "string"
    ? navigationEntry.type
    : undefined
  return createScrollRestoration({
    navigationType,
    get fullPath() { return `${location.pathname}${location.search}${location.hash}` },
    get scrollY() { return window.scrollY },
    get innerHeight() { return window.innerHeight },
    get scrollHeight() { return document.documentElement.scrollHeight },
    storage: createSafeStorage(() => sessionStorage),
    addEventListener: (type, listener, options) => window.addEventListener(type, listener, options),
    removeEventListener: (type, listener, options) => window.removeEventListener(type, listener, options),
    setTimeout: (callback, delay) => window.setTimeout(callback, delay),
    clearTimeout: (handle) => window.clearTimeout(handle),
    nextTick,
    images: () => Array.from(document.images),
    observeDocumentResize: (callback) => {
      if (typeof ResizeObserver === "undefined") return () => undefined
      const observer = new ResizeObserver(callback)
      observer.observe(document.documentElement)
      return () => observer.disconnect()
    },
    restore: (top) => {
      const previousBehavior = document.documentElement.style.scrollBehavior
      document.documentElement.style.scrollBehavior = "auto"
      window.scrollTo({ top, behavior: "auto" })
      document.documentElement.style.scrollBehavior = previousBehavior
    },
  })
}

/** 应用生命周期内唯一的滚动恢复服务。 */
export const scrollRestoration = createBrowserScrollRestoration()
