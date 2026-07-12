import { inject, type InjectionKey } from "vue"
import { createArticleApi } from "@/request/api/article"
import { createArticleTypeApi } from "@/request/api/article-type"
import { createTagApi } from "@/request/api/tag"
import type { HttpTransport } from "@/request/transport"

/** 应用范围内复用的类型化 API 服务容器。 */
export type ApiServices = {
  /** 文章 API 适配器。 */ readonly article: ReturnType<typeof createArticleApi>
  /** 文章分类 API 适配器。 */ readonly articleType: ReturnType<typeof createArticleTypeApi>
  /** 标签 API 适配器。 */ readonly tag: ReturnType<typeof createTagApi>
}

/** Vue 组件树中的 API 服务容器键。 */
export const apiServicesKey: InjectionKey<ApiServices> = Symbol("api-services")

/** 使用显式传入的传输层和服务地址创建一次 API 服务容器。 */
export function createApiServices(client: HttpTransport, serverUrl: string): ApiServices {
  return {
    article: createArticleApi(client, serverUrl),
    articleType: createArticleTypeApi(client, serverUrl),
    tag: createTagApi(client),
  }
}

/** 读取应用挂载时提供的 API 服务容器。 */
export function useApiServices(): ApiServices {
  const services = inject(apiServicesKey)
  if (services === undefined) throw new Error("缺少 API 服务提供者")
  return services
}

