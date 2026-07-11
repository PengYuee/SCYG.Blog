<script setup lang="ts">
import { ArrowPathIcon, BookOpenIcon, ChatBubbleLeftRightIcon, CodeBracketIcon, CodeBracketSquareIcon } from "@heroicons/vue/24/outline"
import { computed, ref, type Component } from "vue"
import { AUTHOR_NAME, PROFILE_LINKS, type ProfileLinkId } from "@/content/profile"
import { pickLocalQuote } from "@/content/quotes"

/** 当前导航周期内稳定的本地名言。 */
const quote = ref(pickLocalQuote())
/** 站外链接使用的 Heroicons 映射。 */
const LINK_ICONS: Record<ProfileLinkId, Component> = { qq: ChatBubbleLeftRightIcon, gitee: CodeBracketSquareIcon, cnblogs: BookOpenIcon, github: CodeBracketIcon }
/** 对模板公开的精确个人链接。 */
const profileLinks = computed(() => PROFILE_LINKS)

/** 用户显式刷新名言，并避免立即重复。 */
const refreshQuote = (): void => {
  quote.value = pickLocalQuote(quote.value.id)
}
</script>

<template>
  <section class="mx-auto flex max-w-3xl flex-col items-center text-center" aria-labelledby="hero-profile-name">
    <img :src="'/images/avatar.jpg'" width="128" height="128" class="size-32 rounded-full border-4 border-[color:var(--color-hero-text-muted)] object-cover shadow-[var(--shadow-dialog)]" :alt="`${AUTHOR_NAME}的头像`" />
    <p class="mt-6 text-sm font-semibold tracking-[0.2em] text-[color:var(--color-hero-text-muted)]">PENGYUEE</p>
    <h1 id="hero-profile-name" class="mt-2 text-balance font-[family-name:var(--font-family-display)] text-[length:var(--font-size-display)] font-bold leading-[var(--line-height-display)] text-[color:var(--color-hero-text)]">{{ AUTHOR_NAME }}</h1>
    <p class="mt-3 text-base text-[color:var(--color-hero-text-muted)]">个人博客</p>
    <nav class="mt-6 flex flex-wrap justify-center gap-3" aria-label="个人站外链接">
      <a v-for="link in profileLinks" :key="link.id" :href="link.href" target="_blank" rel="noopener noreferrer" class="hero-profile-link inline-flex min-h-11 items-center gap-2 rounded-full border px-4 text-sm font-medium">
        <component :is="LINK_ICONS[link.id]" class="size-5" aria-hidden="true" />
        {{ link.label }}
      </a>
    </nav>
    <figure class="mt-8 max-w-2xl text-[color:var(--color-hero-text)]">
      <blockquote data-testid="hero-quote" class="text-pretty font-[family-name:var(--font-family-display)] text-xl leading-relaxed">“{{ quote.text }}”</blockquote>
      <figcaption class="mt-2 text-sm text-[color:var(--color-hero-text-muted)]">{{ quote.source }}</figcaption>
    </figure>
    <button data-testid="quote-refresh" type="button" class="hero-quote-button mt-3 inline-flex min-h-11 items-center gap-2 rounded-full px-4 text-sm font-semibold" @click="refreshQuote">
      <ArrowPathIcon class="size-4" aria-hidden="true" />
      换一句
    </button>
  </section>
</template>

<style scoped>
.hero-profile-link {
  color: var(--color-hero-text);
  border-color: color-mix(in srgb, var(--color-hero-text) 30%, transparent);
  background: color-mix(in srgb, var(--color-hero-text) 10%, transparent);
  backdrop-filter: blur(var(--space-3));
}
.hero-profile-link:hover {
  border-color: var(--color-hero-text-muted);
  background: color-mix(in srgb, var(--color-hero-text) 20%, transparent);
}
.hero-quote-button { color: var(--color-hero-text-muted); }
.hero-quote-button:hover {
  color: var(--color-hero-text);
  background: color-mix(in srgb, var(--color-hero-text) 10%, transparent);
}
</style>