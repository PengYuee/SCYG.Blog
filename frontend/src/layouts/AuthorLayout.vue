<script setup lang="ts">
import { RouterLink, RouterView, useRouter } from "vue-router"
import { useAuthSession } from "@/services/auth-session"

const router = useRouter()
const authSession = useAuthSession()

/** 清除本地短期令牌并返回公共首页。 */
const logout = async (): Promise<void> => {
  authSession.logout()
  await router.replace("/")
}

</script>

<template>
  <div data-layout="author" class="min-h-[100dvh] bg-canvas text-text-primary">
    <a href="#author-main" class="sr-only focus:not-sr-only">跳到写作区域</a>
    <header class="border-b border-border-subtle bg-surface">
      <nav class="mx-auto flex h-[var(--layout-nav-height)] max-w-[var(--layout-container)] items-center justify-between px-6" aria-label="作者导航">
        <RouterLink to="/" class="font-display text-xl font-semibold">SCYG 写作台</RouterLink>
        <div class="flex items-center gap-2 text-sm">
          <RouterLink class="rounded-lg px-4 py-3 hover:bg-surface-hover" to="/author/articles/new">新文章</RouterLink>
          <RouterLink class="rounded-lg px-4 py-3 hover:bg-surface-hover" to="/author/taxonomy">分类与标签</RouterLink>
          <button type="button" class="rounded-lg px-4 py-3 hover:bg-surface-hover" @click="logout">退出登录</button>
        </div>
      </nav>
    </header>
    <main id="author-main" class="mx-auto max-w-[var(--layout-container)] px-6 py-8"><RouterView /></main>
  </div>
</template>
