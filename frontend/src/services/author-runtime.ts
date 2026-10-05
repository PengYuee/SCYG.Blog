import { createFakeAuthorRepositories } from "@/services/fake-author"
import { createMutationGuard } from "@/services/mutation-guard"
import type { ApiServices } from "@/request/api-services"
import type { AuthorArticleRepository, AuthorTaxonomyRepository } from "@/services/author-contracts"
import type { AuthState } from "@/stores/auth"

/** 作者运行时依赖。 */
export type AuthorRuntime = { readonly articles: AuthorArticleRepository; readonly taxonomy: AuthorTaxonomyRepository; readonly guard: ReturnType<typeof createMutationGuard> }

const testAuthenticatedState: AuthState = { kind: "authenticated", session: { accessToken: "test-token", tokenType: "Bearer", expiresAt: "2099-01-01T00:00:00Z" } }

/** 创建只供测试注入的内存作者运行时。 */
export function createFakeAuthorRuntime(): AuthorRuntime {
  const repositories = createFakeAuthorRepositories()
  return { articles: repositories.articles, taxonomy: repositories.taxonomy, guard: createMutationGuard(() => testAuthenticatedState) }
}

/** 为已认证作者页面创建真实 API 运行时。 */
export function createAuthorRuntime(services: ApiServices, currentState: () => AuthState): AuthorRuntime {
  return {
    articles: {
      detail: services.article.manageDetail,
      create: services.article.create,
      update: services.article.update,
      uploadImage: services.articleImage.uploadImage,
      deleteImage: services.articleImage.deleteImage,
    },
    taxonomy: {
      /** 分类读取、创建、修改与删除使用真实 v1 适配器。 */
      listArticleTypes: services.articleType.list,
      createArticleType: services.articleType.create,
      updateArticleType: services.articleType.update,
      deleteArticleType: services.articleType.delete,
      /** 标签读取、创建、修改与删除全部使用真实 v1 适配器。 */
      listTags: services.tag.list,
      createTag: services.tag.create,
      updateTag: services.tag.update,
      deleteTag: services.tag.delete,
    },
    guard: createMutationGuard(currentState),
  }
}
