import { z } from "zod"
import { createFakeAuthorRepositories } from "@/services/fake-author"
import { createMutationGuard } from "@/services/mutation-guard"
import type { ApiServices } from "@/request/api-services"
import type { AuthorArticleRepository, AuthorTaxonomyRepository } from "@/services/author-contracts"
import type { AuthClaims } from "@/types/auth"
import { parseAuthRuntimeConfig, type AuthState } from "@/stores/auth"

/** 作者运行时依赖。 */
export type AuthorRuntime = { readonly articles: AuthorArticleRepository; readonly taxonomy: AuthorTaxonomyRepository; readonly guard: ReturnType<typeof createMutationGuard> }

/** 当前部署不允许构造可信作者运行时。 */
export class AuthorRuntimeUnavailableError extends Error {
  /** 稳定错误名称。 */ readonly name = "AuthorRuntimeUnavailableError"
  /** 创建不泄露伪造身份的中文运行时错误。 */
  constructor() { super("当前环境未启用可信作者运行时") }
}

const fakeFlagSchema = z.enum(["true", "false"]).default("false")
const fakeClaims: AuthClaims = { roles: ["author"], permissions: ["article:write"] }
const fakeState: AuthState = { kind: "authenticated", source: "fake", claims: fakeClaims }

/** 仅显式 VITE_FAKE_AUTHOR=true 且非生产时启用作者演示。 */
export const fakeAuthorEnabled = import.meta.env.MODE !== "production" && fakeFlagSchema.parse(import.meta.env["VITE_FAKE_AUTHOR"]) === "true"

/** 创建显式 Fake 作者运行时；生产路由不会调用此工厂。 */
export function createFakeAuthorRuntime(): AuthorRuntime {
  const repositories = createFakeAuthorRepositories()
  const config = parseAuthRuntimeConfig({ mode: import.meta.env.MODE === "test" ? "test" : "development", fakeAuthEnabled: true })
  return { articles: repositories.articles, taxonomy: repositories.taxonomy, guard: createMutationGuard(config, () => fakeState) }
}

/** 为显式 development 可信作者页面创建真实文章与图片运行时。 */
export function createAuthorRuntime(services: ApiServices): AuthorRuntime {
  if (import.meta.env.MODE !== "development" || !fakeAuthorEnabled) throw new AuthorRuntimeUnavailableError()
  const fake = createFakeAuthorRepositories()
  const config = parseAuthRuntimeConfig({ mode: "development", fakeAuthEnabled: true })
  return {
    articles: {
      detail: services.article.detail,
      create: services.article.create,
      update: services.article.update,
      uploadImage: services.articleImage.uploadImage,
      deleteImage: services.articleImage.deleteImage,
    },
    taxonomy: {
      ...fake.taxonomy,
      listArticleTypes: services.articleType.list,
      listTags: services.tag.list,
    },
    guard: createMutationGuard(config, () => fakeState),
  }
}
