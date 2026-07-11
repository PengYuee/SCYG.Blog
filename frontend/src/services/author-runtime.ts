import { z } from "zod"
import { createFakeAuthorRepositories } from "@/services/fake-author"
import { createMutationGuard } from "@/services/mutation-guard"
import type { AuthorArticleRepository, AuthorTaxonomyRepository } from "@/services/author-contracts"
import type { AuthClaims } from "@/types/auth"
import { parseAuthRuntimeConfig, type AuthState } from "@/stores/auth"

/** 作者运行时依赖。 */
export type AuthorRuntime = { readonly articles: AuthorArticleRepository; readonly taxonomy: AuthorTaxonomyRepository; readonly guard: ReturnType<typeof createMutationGuard> }

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
