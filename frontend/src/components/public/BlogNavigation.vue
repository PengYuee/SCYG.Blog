<script setup lang="ts">
import { ArrowRightEndOnRectangleIcon, HomeIcon, NewspaperIcon, PencilSquareIcon } from "@heroicons/vue/24/outline"
import { computed, inject, type Component } from "vue"
import { RouterLink, useRoute } from "vue-router"
import { authSessionKey } from "@/services/auth-session"

/** 公共导航所在的语义表面或固定 Hero 覆盖层。 */
type NavigationTone = "hero" | "overlay" | "surface"
/** 公共导航目标定义。 */
type NavigationTarget = {
  /** 稳定目标标识。 */ readonly id: "home" | "articles" | "author-write" | "login"
  /** 规范完整路径。 */ readonly to: "/" | "/articles" | "/author/articles/new" | "/login"
  /** 中文可见标签。 */ readonly label: string
  /** 对应的 Heroicons 图标。 */ readonly icon: Component
  /** 根据当前路径判断活动态。 */ readonly isActive: (path: string) => boolean
}

/** 公共导航输入属性。 */
withDefaults(defineProps<{ readonly tone?: NavigationTone }>(), { tone: "surface" })

const AUTHOR_TARGET = { id: "author-write", to: "/author/articles/new", label: "写作台", icon: PencilSquareIcon, isActive: (path: string) => path.startsWith("/author") } as const satisfies NavigationTarget
const LOGIN_TARGET = { id: "login", to: "/login", label: "登录", icon: ArrowRightEndOnRectangleIcon, isActive: (path: string) => path === "/login" } as const satisfies NavigationTarget

const authSession = inject(authSessionKey, null)
/** Hero 与固定头部共享导航，并随真实认证状态切换登录或写作入口。 */
const navTargets = computed<readonly NavigationTarget[]>(() => [
  { id: "home", to: "/", label: "首页", icon: HomeIcon, isActive: (path: string) => path === "/" },
  { id: "articles", to: "/articles", label: "文章", icon: NewspaperIcon, isActive: (path: string) => path.startsWith("/articles") },
  authSession?.currentState().kind === "authenticated" ? AUTHOR_TARGET : LOGIN_TARGET,
])
/** 当前路由用于导航活动态和同路由交互。 */
const route = useRoute()

/** 返回目标是否对应当前路径。 */
const isTargetActive = (target: NavigationTarget): boolean => target.isActive(route.path)

/** 返回活动目标的页面语义，非活动目标不输出属性。 */
const ariaCurrent = (target: NavigationTarget): "page" | undefined => isTargetActive(target) ? "page" : undefined

/**
 * 同一完整路由的无修饰主键点击回到页面顶部，其他点击保留 RouterLink 原生语义。
 * @param event 路由链接的鼠标点击事件。
 * @param target 链接对应的规范目标。
 */
const handleNavigationClick = (event: MouseEvent, target: NavigationTarget): void => {
  const isPrimaryClick = event.button === 0
  const hasModifier = event.ctrlKey || event.metaKey || event.shiftKey || event.altKey
  if (isPrimaryClick && !hasModifier && route.fullPath === target.to) window.scrollTo({ top: 0 })
}
</script>

<template>
  <nav class="public-navigation flex items-center gap-2" :class="`public-navigation--${tone}`" aria-label="主要导航">
    <RouterLink
      v-for="target in navTargets"
      :key="target.id"
      :to="target.to"
      class="navigation-link relative inline-flex min-h-11 items-center gap-2 rounded-[var(--radius-control)] px-4 text-sm font-semibold"
      :class="isTargetActive(target) ? 'after:absolute after:inset-x-4 after:bottom-2 after:h-0.5 after:bg-accent' : ''"
      :aria-current="ariaCurrent(target)"
      @click="handleNavigationClick($event, target)"
    >
      <component :is="target.icon" class="size-5" aria-hidden="true" />
      {{ target.label }}
    </RouterLink>
  </nav>
</template>

<style scoped>
.public-navigation--hero {
  padding: var(--space-1);
  border: var(--border-width) solid var(--color-hero-border);
  border-radius: var(--radius-round);
  background: var(--color-hero-surface);
  color: var(--color-hero-text);
  box-shadow: var(--shadow-card);
  backdrop-filter: blur(var(--space-3));
}
.public-navigation--overlay { color: var(--color-hero-text); }
.public-navigation--surface { color: var(--color-text-primary); }
.public-navigation--surface .navigation-link:hover { background: var(--color-surface-hover); }
.public-navigation--hero .navigation-link:hover { background: color-mix(in srgb, var(--color-hero-text) 10%, transparent); }
.public-navigation--hero .navigation-link:active { background: color-mix(in srgb, var(--color-hero-text) 16%, transparent); }
.public-navigation--overlay .navigation-link:hover { background: var(--color-hero-surface-hover); }
.public-navigation--overlay .navigation-link:active { background: var(--color-hero-surface-active); }
</style>
