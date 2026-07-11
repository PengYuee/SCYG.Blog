/** 文章分类。 */
export type ArticleType = {
  /** 分类标识。 */ readonly id: number
  /** 分类名称。 */ readonly name: string
  /** 可选图片地址。 */ readonly imageUrl: string | null
  /** 菜单顺序。 */ readonly menu: number
}

/** 分类创建请求。 */
export type ArticleTypeCreate = { readonly name: string; readonly image: string | null; readonly menu: number }

/** 文章标签。 */
export type Tag = { readonly id: number; readonly name: string }
