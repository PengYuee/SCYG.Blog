/** T2 自包含解析结果，不定义后续生产 API 合同。 */
export type T2ParseResult =
  | { readonly kind: "valid"; readonly title: string }
  | { readonly kind: "malformed" }

/** T2 fixture：在测试边界解析未知输入，专门证明畸形输入分支可测试。 */
export const parseT2Fixture = (input: unknown): T2ParseResult => {
  if (typeof input !== "object" || input === null || !("title" in input)) {
    return { kind: "malformed" }
  }
  const title = input.title
  return typeof title === "string" ? { kind: "valid", title } : { kind: "malformed" }
}

/** T2 fixture 路由表，仅用于证明缺失路由失败，不实现生产路由。 */
const T2_ROUTES = new Map<string, string>([["/t2-smoke", "ready"]])

/** T2 fixture：返回隔离测试路由的二元可观察结果。 */
export const resolveT2Route = (path: string): "ready" | "missing" =>
  T2_ROUTES.has(path) ? "ready" : "missing"
