import type { ArticleCreateRequest, ArticleDetail, ArticleUpdateRequest } from "@/types/article"
import type { ArticleType, ArticleTypeCreate, ArticleTypeUpdateRequest, Tag, TagUpdateRequest } from "@/types/taxonomy"

/** 上传成功且仍待文章提交的正文图片。 */
export type UploadedArticleImage = {
  /** 服务端图片标识，取消操作唯一使用此字段。 */ readonly id: string
  /** 可插入 Markdown 的远程图片地址。 */ readonly url: string
  /** 服务端兜底清理的 UTC 过期时间。 */ readonly expiresAt: string
}

/** 作者写入使用的版本化文章分类资源。 */
export type AuthorArticleType = ArticleType & {
  /** 当前资源版本，用于并发删除。 */ readonly version: number
}

/** 删除文章分类所需的最小并发控制目标。 */
export type ArticleTypeDeleteTarget = {
  /** 待删除的分类标识。 */ readonly id: number
  /** 待删除资源的当前版本。 */ readonly version: number
}

/** 作者写入使用的版本化标签资源。 */
export type AuthorTag = Tag & {
  /** 当前资源版本，用于并发删除。 */ readonly version: number
}

/** 删除标签所需的最小并发控制目标。 */
export type TagDeleteTarget = {
  /** 待删除的标签标识。 */ readonly id: number
  /** 待删除资源的当前版本。 */ readonly version: number
}

/** 作者文章仓储契约，真实与 Fake 适配器共享。 */
export type AuthorArticleRepository = {
  /** 读取管理端文章详情。 */ readonly detail: (id: number) => Promise<ArticleDetail>
  /** 创建文章并返回服务端资源。 */ readonly create: (request: ArticleCreateRequest) => Promise<ArticleDetail>
  /** 局部更新文章并返回服务端资源。 */ readonly update: (request: ArticleUpdateRequest) => Promise<ArticleDetail>
  /** 上传文章图片。 */ readonly uploadImage: (image: File) => Promise<UploadedArticleImage>
  /** 按服务端图片标识删除未提交图片。 */ readonly deleteImage: (id: string) => Promise<boolean>
}

/** 作者分类仓储契约。 */
export type AuthorTaxonomyRepository = {
  /** 读取带版本的分类。 */ readonly listArticleTypes: () => Promise<readonly AuthorArticleType[]>
  /** 读取带版本的标签。 */ readonly listTags: () => Promise<readonly AuthorTag[]>
  /** 创建分类并返回服务端版本化资源。 */ readonly createArticleType: (request: ArticleTypeCreate) => Promise<AuthorArticleType>
  /** 按资源当前版本更新分类名称。 */ readonly updateArticleType: (request: ArticleTypeUpdateRequest) => Promise<AuthorArticleType>
  /** 按资源当前版本删除分类。 */ readonly deleteArticleType: (target: ArticleTypeDeleteTarget) => Promise<void>
  /** 创建标签并返回服务端版本化资源。 */ readonly createTag: (name: string) => Promise<AuthorTag>
  /** 按资源当前版本更新标签名称。 */ readonly updateTag: (request: TagUpdateRequest) => Promise<AuthorTag>
  /** 按资源当前版本删除标签。 */ readonly deleteTag: (target: TagDeleteTarget) => Promise<void>
}
