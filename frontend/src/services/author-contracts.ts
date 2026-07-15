import type { ArticleDetail, ArticleUpdate, ArticleWrite } from "@/types/article"
import type { ArticleType, ArticleTypeCreate, Tag } from "@/types/taxonomy"

/** 上传成功且仍待文章提交的正文图片。 */
export type UploadedArticleImage = {
  /** 服务端图片标识，取消操作唯一使用此字段。 */ readonly id: string
  /** 可插入 Markdown 的远程图片地址。 */ readonly url: string
  /** 服务端兜底清理的 UTC 过期时间。 */ readonly expiresAt: string
}

/** 作者文章仓储契约，真实与 Fake 适配器共享。 */
export type AuthorArticleRepository = {
  /** 读取文章详情。 */ readonly detail: (id: number) => Promise<ArticleDetail>
  /** 创建文章。 */ readonly create: (request: ArticleWrite) => Promise<boolean>
  /** 更新文章。 */ readonly update: (request: ArticleUpdate) => Promise<boolean>
  /** 上传文章图片。 */ readonly uploadImage: (image: File) => Promise<UploadedArticleImage>
  /** 按服务端图片标识删除未提交图片。 */ readonly deleteImage: (id: string) => Promise<boolean>
}

/** 作者分类仓储契约。 */
export type AuthorTaxonomyRepository = {
  /** 读取分类。 */ readonly listArticleTypes: () => Promise<readonly ArticleType[]>
  /** 读取标签。 */ readonly listTags: () => Promise<readonly Tag[]>
  /** 创建分类。 */ readonly createArticleType: (request: ArticleTypeCreate) => Promise<boolean>
  /** 删除分类。 */ readonly deleteArticleType: (id: number) => Promise<boolean>
  /** 创建标签。 */ readonly createTag: (name: string) => Promise<boolean>
  /** 删除标签。 */ readonly deleteTag: (id: number) => Promise<boolean>
}
