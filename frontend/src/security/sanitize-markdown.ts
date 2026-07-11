import createDOMPurify from "dompurify"
import type { Config, DOMPurify } from "dompurify"

/** Markdown 渲染所需的最小 HTML 标签白名单。 */
const MARKDOWN_TAGS = [
  "a", "blockquote", "br", "code", "del", "div", "em", "h1", "h2", "h3", "h4", "h5", "h6", "hr", "img", "input", "li", "ol", "p", "pre", "s", "span", "strong", "table", "tbody", "td", "th", "thead", "tr", "ul",
] as const

/** Markdown 渲染所需的最小属性白名单；事件和样式属性不在此边界内。 */
const MARKDOWN_ATTRIBUTES = ["alt", "checked", "class", "disabled", "href", "id", "rel", "src", "target", "title", "type"] as const

/** DOMPurify 的共享 Markdown 安全策略。 */
export const MARKDOWN_SANITIZE_POLICY: Readonly<Config> = {
  ALLOWED_TAGS: [...MARKDOWN_TAGS],
  ALLOWED_ATTR: [...MARKDOWN_ATTRIBUTES],
  FORBID_TAGS: ["iframe", "script", "style", "template"],
  FORBID_ATTR: ["srcdoc", "style"],
  ALLOW_DATA_ATTR: false,
  ALLOW_ARIA_ATTR: false,
} as const

/** Markdown sanitizer 在无 DOM 环境被执行时的边界错误。 */
class MarkdownSanitizerUnavailableError extends Error {
  /** 创建可诊断的 DOM 环境错误。 */
  public constructor() {
    super("Markdown sanitization requires a browser Window")
    this.name = "MarkdownSanitizerUnavailableError"
  }
}

/**
 * 判断链接是否属于 Markdown 阅读界面允许的地址。
 * @param value 未受信任的 href 属性值。
 * @returns HTTP(S)、mailto、站内相对地址或页内锚点是否安全。
 */
const isSafeLink = (value: string): boolean => {
  const location = value.trim()
  if (location.length === 0 || /[\\\u0000-\u0020\u007f]/u.test(location) || location.startsWith("//")) return false
  if (/^(?:https?:\/\/|mailto:|#|\?|\/(?!\/)|\.\.?\/)/iu.test(location)) return true
  return !/^[a-z][a-z\d+.-]*:/iu.test(location) && /^[\p{L}\p{N}_~%-][^\\\s:]*$/u.test(location)
}

/**
 * 判断图片是否属于 Markdown 阅读界面允许的地址。
 * @param value 未受信任的 src 属性值。
 * @returns HTTP(S) 或明确的站内相对图片地址是否安全。
 */
const isSafeImage = (value: string): boolean => !/[\\\u0000-\u001f\u007f]/u.test(value) && /^(?:https?:\/\/|\/(?!\/)|\.\.?\/)/iu.test(value.trim())

/**
 * 约束 DOMPurify 保留下来的 URL、外链隔离属性和标题锚点。
 * @param node 当前已完成基础属性清理的 DOM 节点。
 */
const enforceMarkdownAttributes = (node: Element): void => {
  if (node instanceof HTMLAnchorElement) {
    const href = node.getAttribute("href")
    if (href === null || !isSafeLink(href)) {
      node.removeAttribute("href")
      node.removeAttribute("target")
      node.removeAttribute("rel")
    } else if (/^https?:\/\//iu.test(href.trim())) {
      node.setAttribute("target", "_blank")
      node.setAttribute("rel", "noopener noreferrer")
    } else {
      node.removeAttribute("target")
      node.removeAttribute("rel")
    }
  }

  if (node instanceof HTMLImageElement) {
    const src = node.getAttribute("src")
    if (src === null || !isSafeImage(src)) node.removeAttribute("src")
  }

  if (/^H[1-6]$/u.test(node.tagName)) {
    const id = node.getAttribute("id")
    if (id !== null && !/^[\p{L}\p{N}_-]+$/u.test(id)) node.removeAttribute("id")
  } else {
    node.removeAttribute("id")
  }
}

/**
 * 创建仅归 Markdown 边界所有的 DOMPurify 实例并注册一次私有 hook。
 * @returns 与包级全局 hook 完全隔离的 sanitizer。
 */
const createMarkdownPurifier = (): DOMPurify => {
  if (typeof window === "undefined") throw new MarkdownSanitizerUnavailableError()
  const purifier = createDOMPurify(window)
  purifier.addHook("afterSanitizeAttributes", enforceMarkdownAttributes)
  return purifier
}

let markdownPurifier: DOMPurify | undefined

/**
 * 清理未受信任的 Markdown 源或 md-editor-v3 生成的 HTML。
 * 同一私有策略同时保护预览输入、预览 sanitize 钩子和目录标题来源。
 * @param untrustedContent 未受信任的 Markdown 或 HTML 字符串。
 * @returns 仅包含 Markdown 阅读所需安全结构的字符串。
 */
export const sanitizeMarkdown = (untrustedContent: string): string => {
  markdownPurifier ??= createMarkdownPurifier()
  return markdownPurifier.sanitize(untrustedContent, MARKDOWN_SANITIZE_POLICY)
}
