import { inject, type InjectionKey } from "vue"
import { RuntimeConfigError, type RuntimeConfig } from "@/config/runtime"

/** Vue 组件树中的运行时配置键。 */
export const runtimeConfigKey: InjectionKey<RuntimeConfig> = Symbol("runtime-config")

/** 读取应用启动时提供的运行时配置。 */
export function useRuntimeConfig(): RuntimeConfig {
  const config = inject(runtimeConfigKey)
  if (config === undefined) throw new RuntimeConfigError("缺少运行时配置提供者", "CONFIG_INVALID", undefined)
  return config
}