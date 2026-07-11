import { writeFile } from "node:fs/promises"
import { describe, expect, it, vi } from "vitest"
import { createMutationGuard } from "@/services/mutation-guard"
import { createAuthStore, parseAuthRuntimeConfig, type AuthState } from "@/stores/auth"
import { authorRouteAvailability } from "@/router/guards"
import { createPinia, setActivePinia } from "pinia"

const happyArtifact = process.env["T5_AUTH_HAPPY_ARTIFACT"]
const blockedArtifact = process.env["T5_AUTH_BLOCKED_ARTIFACT"]

describe.skipIf(happyArtifact === undefined || blockedArtifact === undefined)("T5 named real-surface QA", () => {
  it("emits fake-author success and three production blocked domains", async () => {
    // Given: explicit test Fake Auth and hostile production configuration.
    setActivePinia(createPinia())
    const claims = { roles: ["author"], permissions: ["article:write"] } as const
    const fakeConfig = parseAuthRuntimeConfig({ mode: "test", fakeAuthEnabled: true })
    const fakeStore = createAuthStore(fakeConfig, claims)()
    const happyAdapter = vi.fn(async () => "article-created")

    // When: the happy fake operation and all production domain operations execute through the shared guard.
    const happyMutation = await createMutationGuard(fakeConfig, () => fakeStore.state).execute("article", happyAdapter)
    const production = parseAuthRuntimeConfig({ mode: "production", fakeAuthEnabled: true })
    const unsupported: AuthState = { kind: "unsupported", reason: "backend auth unavailable" }
    const articleAdapter = vi.fn(async () => "article-created")
    const taxonomyAdapter = vi.fn(async () => "category-deleted")
    const imageAdapter = vi.fn(async () => "image-uploaded")
    const productionGuard = createMutationGuard(production, () => unsupported)
    const article = await productionGuard.execute("article", articleAdapter)
    const taxonomy = await productionGuard.execute("taxonomy", taxonomyAdapter)
    const image = await productionGuard.execute("image", imageAdapter)

    // Then: named JSON captures truthful route/capability outcomes and exact invocation counts.
    const happy = {
      scenario: "fake-author-success",
      result: happyMutation,
      adapterInvocations: happyAdapter.mock.calls.length,
      canAuthor: fakeStore.canAuthor,
      canAdmin: fakeStore.canAdmin,
      route: authorRouteAvailability(fakeConfig, fakeStore.state),
    }
    const blocked = {
      scenario: "production-three-domain-block",
      effectiveFakeAuthEnabled: production.fakeAuthEnabled,
      route: authorRouteAvailability(production, unsupported),
      domains: {
        article: { result: article, adapterInvocations: articleAdapter.mock.calls.length },
        taxonomy: { result: taxonomy, adapterInvocations: taxonomyAdapter.mock.calls.length },
        image: { result: image, adapterInvocations: imageAdapter.mock.calls.length },
      },
    }
    await writeFile(happyArtifact ?? "", `${JSON.stringify(happy, null, 2)}\n`, "utf8")
    await writeFile(blockedArtifact ?? "", `${JSON.stringify(blocked, null, 2)}\n`, "utf8")
    expect(happy).toMatchObject({ result: { ok: true, value: "article-created" }, adapterInvocations: 1, canAuthor: true, canAdmin: false, route: { kind: "available" } })
    expect(blocked.domains.article.adapterInvocations).toBe(0)
    expect(blocked.domains.taxonomy.adapterInvocations).toBe(0)
    expect(blocked.domains.image.adapterInvocations).toBe(0)
  })
})
