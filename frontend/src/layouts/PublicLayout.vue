<script setup lang="ts">
import { ref } from "vue"
import BlogFooter from "@/components/public/BlogFooter.vue"
import BlogHeader from "@/components/public/BlogHeader.vue"

/** 非首页公共布局输入属性。 */
type Props = {
  /** 公共短 Hero 中唯一的页面主标题。 */ readonly title: string
  /** 主要内容锚点标识。 */ readonly contentId?: string
  /** 跳过导航链接的中文标签。 */ readonly skipLabel?: string
}

withDefaults(defineProps<Props>(), {
  contentId: "blog-content",
  skipLabel: "跳到主要内容",
})
/** 公共短 Hero 的可观察边界。 */
const heroBoundary = ref<HTMLElement | null>(null)
</script>

<template>
  <div data-layout="public" class="min-h-[100dvh] bg-canvas text-text-primary">
    <a :href="`#${contentId}`" class="fixed left-4 top-4 z-[60] -translate-y-24 rounded-lg bg-surface px-4 py-3 font-semibold text-accent shadow-[var(--shadow-nav)] focus:translate-y-0">{{ skipLabel }}</a>
    <BlogHeader :boundary="heroBoundary" public-overlay />
    <section ref="heroBoundary" class="blog-hero public-short-hero flex items-center justify-center pt-[var(--layout-nav-height)]" :aria-labelledby="`${contentId}-title`">
      <h1 :id="`${contentId}-title`" class="text-balance font-[family-name:var(--font-family-display)] text-[length:var(--font-size-h1)] font-bold leading-[var(--line-height-h1)] text-hero-text">{{ title }}</h1>
    </section>
    <!-- 正文与页脚共同占满动态视口，正文弹性填充剩余高度并避开固定头部。 -->
    <div class="flex min-h-[100dvh] flex-col">
      <main :id="contentId" class="flex-1 pb-[var(--space-16)] pt-[calc(var(--layout-nav-height)+var(--space-10))]" tabindex="-1">
        <div class="blog-shell"><slot /></div>
      </main>
      <BlogFooter />
    </div>
  </div>
</template>

<style scoped>
/* 已批准的非首页短 Hero 保持正常文档流，并覆盖全高 Hero 原语的最小高度。 */
.public-short-hero {
  height: 20vh;
  min-height: 20vh;
}
</style>
