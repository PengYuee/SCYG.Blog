import "@/assets/main.css"
import { bootstrapApplication } from "@/bootstrap"
import { initializeTheme } from "@/theme/theme"

/** 在挂载应用前同步主题状态和浏览器栏颜色。 */
initializeTheme()

/** 在主题与样式就绪后启动运行时配置边界。 */
void bootstrapApplication()
