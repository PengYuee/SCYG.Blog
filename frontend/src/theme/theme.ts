import { readonly, ref } from "vue"

/** 应用支持的完整主题集合。 */
const THEMES = ["night", "sky"] as const

/** 可持久化的主题标识。 */
export type Theme = (typeof THEMES)[number]

/** 浏览器本地主题偏好的唯一存储键，必须与 index.html 预初始化脚本保持一致。 */
export const THEME_STORAGE_KEY = "scyg-blog-theme"

/** 全局唯一的响应式主题状态，所有组件共享同一状态源。 */
const activeTheme = ref<Theme>("night")

/** 供组件读取的当前主题。 */
export const currentTheme = readonly(activeTheme)

/** 严格解析不受信任的持久化值，只接受已声明主题。 */
const parseTheme = (value: string | null): Theme | null => {
  if (value === "night" || value === "sky") return value
  return null
}

/** 安静读取用户偏好；隐私模式或存储禁用时回退到系统设置。 */
const readStoredTheme = (): Theme | null => {
  try {
    return parseTheme(window.localStorage.getItem(THEME_STORAGE_KEY))
  } catch {
    return null
  }
}

/** 无有效用户偏好时，将系统深色偏好映射到夜空主题。 */
const resolveInitialTheme = (): Theme => {
  const storedTheme = readStoredTheme()
  if (storedTheme !== null) return storedTheme
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "night" : "sky"
}

/** 从已生效的主题 CSS 读取浏览器栏颜色，避免在 TypeScript 重复主题色值。 */
const syncBrowserThemeColor = (): void => {
  const themeColor = window.getComputedStyle(document.documentElement).getPropertyValue("--color-browser-theme").trim()
  const meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')
  if (meta !== null && themeColor.length > 0) meta.content = themeColor
}

/** 同步应用主题到根节点、响应式状态和浏览器栏颜色。 */
export const applyTheme = (theme: Theme): void => {
  document.documentElement.dataset["theme"] = theme
  activeTheme.value = theme
  syncBrowserThemeColor()
}

/** 在 Vue 应用启动前采用持久化偏好或系统偏好。 */
export const initializeTheme = (): void => {
  applyTheme(resolveInitialTheme())
}

/** 在两个受支持主题间切换，并尽力持久化用户的显式选择。 */
export const toggleTheme = (): void => {
  const nextTheme: Theme = activeTheme.value === "night" ? "sky" : "night"
  try {
    window.localStorage.setItem(THEME_STORAGE_KEY, nextTheme)
  } catch {
    // 存储不可用不应阻止当前页面切换主题。
    return applyTheme(nextTheme)
  }
  applyTheme(nextTheme)
}
