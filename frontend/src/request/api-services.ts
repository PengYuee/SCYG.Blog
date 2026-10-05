import { inject, type InjectionKey } from "vue"
import { createAuthApi, type AuthApi } from "@/request/api/auth"
import { createArticleApi, type ArticleApi } from "@/request/api/article"
import { createArticleImageApi, type ArticleImageApi } from "@/request/api/article-image"
import { createArticleTypeApi, type ArticleTypeApi } from "@/request/api/article-type"
import { createPublicTaxonomyApi, type PublicTaxonomyApi } from "@/request/api/public-taxonomy"
import { createTagApi, type TagApi } from "@/request/api/tag"
import type { HttpTransport } from "@/request/transport"

export type ApiServices = {
  /** 用户名密码登录 API。 */ readonly auth: AuthApi
  /** 文章 API 适配器。 */ readonly article: ArticleApi
  /** 正文图片 API 适配器。 */ readonly articleImage: ArticleImageApi
  /** 管理端文章分类 API 适配器。 */ readonly articleType: ArticleTypeApi
  /** 管理端标签 API 适配器。 */ readonly tag: TagApi
  /** 公开文章分类和标签只读 API 适配器。 */ readonly publicTaxonomy: PublicTaxonomyApi
}

/** Vue 组件树中的 API 服务容器键。 */
export const apiServicesKey: InjectionKey<ApiServices> = Symbol("api-services")

/** 使用显式传入的传输层和服务地址创建一次 API 服务容器。 */
export function createApiServices(client: HttpTransport, serverUrl: string): ApiServices {
  return {
    auth: createAuthApi(client),
    article: createArticleApi(client),
    articleImage: createArticleImageApi(client, serverUrl),
    articleType: createArticleTypeApi(client, serverUrl),
    tag: createTagApi(client),
    publicTaxonomy: createPublicTaxonomyApi(client, serverUrl),
  }
}

/** 读取应用挂载时提供的 API 服务容器。 */
export function useApiServices(): ApiServices {
  const services = inject(apiServicesKey)
  if (services === undefined) throw new Error("缺少 API 服务提供者")
  return services
}
