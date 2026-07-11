import { enableAutoUnmount } from "@vue/test-utils"
import { afterEach } from "vitest"

/** 将 Vue Test Utils 的卸载生命周期绑定到 Vitest，阻止 DOM 状态跨测试泄漏。 */
enableAutoUnmount(afterEach)
