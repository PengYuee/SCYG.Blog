import { loadRuntimeConfig, RuntimeConfigError, type RuntimeConfig } from "@/config/runtime"
import { configureHttp } from "@/request/http"

/** Vue 应用动态模块的最小契约。 */
type ApplicationModule = {
  readonly mountApplication: (config: RuntimeConfig) => void
}

/** 启动边界可替换依赖。 */
type BootstrapDependencies = {
  readonly fetcher?: typeof fetch
  readonly loadApplication?: () => Promise<ApplicationModule>
  readonly document?: Document
}

/** 配置失败时向用户展示的稳定消息。 */
export const STARTUP_CONFIG_ERROR_MESSAGE = "运行时配置加载失败，请检查 config.json 后重试。"

/** 先加载配置并初始化 HTTP，再导入和挂载业务应用。 */
export async function bootstrapApplication(dependencies: BootstrapDependencies = {}): Promise<void> {
  const fetcher = dependencies.fetcher ?? fetch
  const loadApplication = dependencies.loadApplication ?? (() => import("@/application"))
  const targetDocument = dependencies.document ?? document
  try {
    const config = await loadRuntimeConfig(fetcher)
    configureHttp(config)
    const application = await loadApplication()
    application.mountApplication(config)
  } catch (error) {
    if (!(error instanceof RuntimeConfigError)) throw error
    const root = targetDocument.querySelector("#app")
    if (root === null) throw error
    const alert = targetDocument.createElement("div")
    alert.setAttribute("role", "alert")
    alert.textContent = STARTUP_CONFIG_ERROR_MESSAGE
    root.replaceChildren(alert)
  }
}