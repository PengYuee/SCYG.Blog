import { enableAutoUnmount } from "@vue/test-utils"
import { afterEach } from "vitest"

/** jsdom 不提供 IntersectionObserver；默认桩只保持生命周期契约，行为测试通过 props 注入回调。 */
if (typeof globalThis.IntersectionObserver === "undefined") {
  class TestIntersectionObserver {
    observe(): void {}
    disconnect(): void {}
  }
  globalThis.IntersectionObserver = TestIntersectionObserver as unknown as typeof IntersectionObserver
}

/** jsdom 不实现布局滚动；组件行为测试只需允许调用。 */
window.scrollTo = (() => undefined) as typeof window.scrollTo

/** 将 Vue Test Utils 的卸载生命周期绑定到 Vitest，阻止 DOM 状态跨测试泄漏。 */
enableAutoUnmount(afterEach)
