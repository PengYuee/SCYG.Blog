/** 文章状态。 */
export type ArticleStatus = 1 | 2 | 3

/** 只读文章领域模型。 */
export type ArticleDetail = {
  /** 文章标识。 */ readonly id: number
  /** 标题。 */ readonly title: string
  /** URL slug。 */ readonly slug: string
  /** 摘要。 */ readonly digest: string
  /** Markdown 源文本。 */ readonly markdown: string
  /** 分类标识。 */ readonly articleTypeId: number
  /** 标签标识。 */ readonly tagIds: readonly number[]
  /** 发布状态。 */ readonly status: ArticleStatus
  /** 点赞数。 */ readonly support: number
  /** 评论数。 */ readonly comment: number
  /** 访问数。 */ readonly visited: number
  /** 乐观锁版本。 */ readonly version: number
  /** 创建时间。 */ readonly createdAt: string
  /** 更新时间。 */ readonly updatedAt: string | null
}

/** 列表文章摘要；当前 API 返回完整资源。 */
export type ArticleSummary = ArticleDetail

/** 旧文章列表请求。 */
export type ArticleListRequest = {
  /** 标签筛选。 */ readonly tagId?: number
  /** 分类筛选。 */ readonly articleTypeId?: number
  /** 分页模型。 */ readonly pageModel: { readonly pageIndex: number; readonly pageSize: number }
}

/** 文章写入字段。 */
export type ArticleWrite = {
  /** 标题。 */ readonly title: string
  /** Markdown 源文本。 */ readonly markdown: string
  /** 摘要。 */ readonly digest: string
  /** 标签标识。 */ readonly tagIds: readonly number[]
  /** 分类标识。 */ readonly articleTypeId: number
}

/** 文章更新请求。 */
export type ArticleUpdate = ArticleWrite & { readonly id: number }
