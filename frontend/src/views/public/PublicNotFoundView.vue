<script setup lang="ts">
import { ArrowLeftIcon, ExclamationTriangleIcon } from "@heroicons/vue/24/outline"
import { computed } from "vue"
import { RouterLink } from "vue-router"

/** 公共失败页展示模式。 */
type FailureMode = "not-found" | "login-unavailable" | "author-unavailable" | "invalid-legacy-id"
/** 公共失败页输入。 */
type Props = { readonly mode?: FailureMode }

const props = withDefaults(defineProps<Props>(), { mode: "not-found" })
/** 按稳定失败模式选择标题。 */
const title = computed(() => ({
  "not-found": "这里没有你要找的页面",
  "login-unavailable": "登录功能暂不可用",
  "author-unavailable": "写作功能暂不可用",
  "invalid-legacy-id": "文章标识无效",
})[props.mode])
/** 按稳定失败模式选择解释。 */
const description = computed(() => ({
  "not-found": "地址可能已变更，也可能从未存在。你可以返回文章列表继续阅读。",
  "login-unavailable": "后端认证能力尚未提供，因此当前版本不会展示虚假的登录流程。",
  "author-unavailable": "后端认证能力尚未提供，因此作者入口被真实标记为不可用。",
  "invalid-legacy-id": "旧写作链接中的 id 必须是正整数，请检查原始链接后重试。",
})[props.mode])
</script>

<template>
  <main data-layout="public" class="flex min-h-[100dvh] items-center justify-center bg-canvas p-8 text-text-primary">
    <section class="blog-card max-w-2xl p-12 text-center" :aria-labelledby="`${mode}-title`">
      <span class="mx-auto inline-flex size-16 items-center justify-center rounded-full bg-error-soft text-error"><ExclamationTriangleIcon class="size-8" aria-hidden="true" /></span>
      <p class="mt-6 text-xs font-semibold uppercase tracking-[0.18em] text-accent">{{ mode === "not-found" ? "404" : "UNAVAILABLE" }}</p>
      <h1 :id="`${mode}-title`" class="mt-2 text-balance font-[family-name:var(--font-family-display)] text-[length:var(--font-size-h1)] font-bold leading-[var(--line-height-h1)]">{{ title }}</h1>
      <p class="mx-auto mt-4 max-w-xl text-pretty text-text-secondary">{{ description }}</p>
      <RouterLink to="/articles" class="mt-8 inline-flex min-h-11 items-center gap-2 rounded-[var(--radius-control)] bg-accent px-6 font-semibold text-[color:var(--color-hero-text)] hover:bg-accent-hover active:scale-[0.98]"><ArrowLeftIcon class="size-5" aria-hidden="true" />返回文章列表</RouterLink>
    </section>
  </main>
</template>
