import { defineStore } from "pinia"
import { readonly, ref } from "vue"
import type { ToastMessage, ToastTone } from "@/types/common"

/** Toast 展示时长，单位为毫秒。 */
const TOAST_DURATION_MS = 6000
/** 同时可见的 Toast 最大数量。 */
const MAX_VISIBLE_TOASTS = 5

/** UI 状态仓库：集中管理全局 Toast。 */
export const useUiStore = defineStore(
  "ui",
  /**
   * 创建 UI 状态与操作。
   * @param 无 此函数无参数。
   * @returns UI 状态与 Toast 操作。
   * @throws 此初始化函数不抛出异常。
   * @since 1.0.0
   */
  () => {
    /** 当前显示的 Toast 列表，由仓库内部有意维护可变状态。 */
    const toasts = ref<readonly ToastMessage[]>([])
    /** Toast 标识单调计数器；递增是其唯一用途，可避免同毫秒重复标识。 */
    let nextToastId = 1
    /** 自动关闭定时器表；可变映射用于在手动关闭或淘汰时释放句柄。 */
    const toastTimers = new Map<number, number>()

    /**
     * 移除指定 Toast 并释放其自动关闭定时器。
     * @param id Toast 唯一标识。
     * @returns 无返回值。
     * @throws 此操作不抛出异常。
     * @since 1.0.0
     */
    const dismissToast = (id: number): void => {
      const timeoutHandle = toastTimers.get(id)
      if (timeoutHandle !== undefined) {
        window.clearTimeout(timeoutHandle)
        toastTimers.delete(id)
      }
      toasts.value = toasts.value.filter(
        /**
         * 保留非目标 Toast。
         * @param toast 当前 Toast。
         * @returns 非目标项返回 true。
         * @throws 此回调不抛出异常。
         * @since 1.0.0
         */
        (toast) => toast.id !== id,
      )
    }

    /**
     * 展示一条全局 Toast 并安排自动关闭。
     * @param tone 消息级别。
     * @param title 消息标题。
     * @param description 可选补充说明。
     * @returns 新 Toast 的唯一标识。
     * @throws 此操作不抛出异常。
     * @since 1.0.0
     */
    const showToast = (tone: ToastTone, title: string, description?: string): number => {
      const id = nextToastId
      nextToastId += 1
      const toast: ToastMessage = description === undefined ? { id, tone, title } : { id, tone, title, description }

      // 达到上限时先完整关闭最旧项，确保其旧定时器不会影响后续 Toast。
      if (toasts.value.length >= MAX_VISIBLE_TOASTS) {
        const oldestToast = toasts.value[0]
        if (oldestToast !== undefined) {
          dismissToast(oldestToast.id)
        }
      }

      toasts.value = [...toasts.value, toast]
      const timeoutHandle = window.setTimeout(
        /**
         * 到期后关闭当前 Toast。
         * @param 无 此回调无参数。
         * @returns 无返回值。
         * @throws 此回调不抛出异常。
         * @since 1.0.0
         */
        () => dismissToast(id),
        TOAST_DURATION_MS,
      )
      toastTimers.set(id, timeoutHandle)
      return id
    }

    return { toasts: readonly(toasts), showToast, dismissToast }
  },
)