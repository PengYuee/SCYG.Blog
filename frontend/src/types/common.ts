/** 通知消息级别。 */
export type ToastTone = "success" | "warning" | "error" | "info"

/** 全局通知消息。 */
export type ToastMessage = {
  /** 消息唯一标识。 */
  readonly id: number
  /** 消息级别。 */
  readonly tone: ToastTone
  /** 消息标题。 */
  readonly title: string
  /** 可选补充说明。 */
  readonly description?: string
}
