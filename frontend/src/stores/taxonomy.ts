import type { ArticleType, Tag } from "@/types/taxonomy"

/** 分类与标签依赖契约。 */
export interface TaxonomyApi {
  /** 获取有序分类字典。 */ listArticleTypes(): Promise<readonly ArticleType[]>
  /** 获取标签字典。 */ listTags(): Promise<readonly Tag[]>
}

/** 分类和标签加载状态。 */
export type TaxonomyState =
  | { readonly kind: "idle" }
  | { readonly kind: "loading" }
  | { readonly kind: "ready"; readonly articleTypes: readonly ArticleType[]; readonly tags: readonly Tag[] }
  | { readonly kind: "empty"; readonly articleTypes: readonly []; readonly tags: readonly [] }
  | { readonly kind: "error"; readonly message: string }

/** 分类状态及加载操作。 */
export interface Taxonomy {
  /** 当前判别联合状态。 */ readonly state: TaxonomyState
  /** 并行加载分类和标签，请求中重复调用会忽略。 */ load(): Promise<void>
  /** 从错误状态重试完整字典请求。 */ retry(): Promise<void>
}

/** 创建 UI 无关、依赖可注入的分类状态机。 */
export function createTaxonomy(api: TaxonomyApi): Taxonomy {
  /** 内部状态由状态转换有意维护可变引用。 */
  let state: TaxonomyState = { kind: "idle" }

  /** 执行一次分类和标签加载。 */
  const load = async (): Promise<void> => {
    if (state.kind === "loading") return
    state = { kind: "loading" }
    try {
      const [articleTypes, tags] = await Promise.all([api.listArticleTypes(), api.listTags()])
      // 两个字典均无数据时使用 empty，任一有数据即是可消费的 ready。
      state = articleTypes.length === 0 && tags.length === 0
        ? { kind: "empty", articleTypes: [], tags: [] }
        : { kind: "ready", articleTypes, tags }
    } catch (error) {
      state = { kind: "error", message: error instanceof Error ? error.message : "Taxonomy request failed" }
    }
  }

  return {
    /** 读取最新状态而不暴露可写引用。 */
    get state() { return state },
    load,
    /** 仅失败状态允许重试，避免重复请求。 */
    async retry() { if (state.kind === "error") await load() },
  }
}
