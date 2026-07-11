<script setup lang="ts">
import { BookOpenIcon, ChatBubbleLeftRightIcon, CodeBracketIcon, CodeBracketSquareIcon } from "@heroicons/vue/24/outline"
import type { Component } from "vue"
import { AUTHOR_NAME, PROFILE_LINKS, type ProfileLinkId } from "@/content/profile"

/** 个人链接简短标签。 */
const LINK_LABELS: Record<ProfileLinkId, string> = { qq: "QQ", gitee: "Gitee", cnblogs: "博客园", github: "GitHub" }
/** 个人链接 Heroicons 映射。 */
const LINK_ICONS: Record<ProfileLinkId, Component> = { qq: ChatBubbleLeftRightIcon, gitee: CodeBracketSquareIcon, cnblogs: BookOpenIcon, github: CodeBracketIcon }
</script>

<template>
  <aside class="blog-card blog-sidebar overflow-hidden" aria-labelledby="profile-card-name">
    <div class="bg-surface-muted p-6 text-center">
      <img :src="'/images/avatar.jpg'" width="112" height="112" class="mx-auto size-28 rounded-full border-4 border-surface object-cover shadow-[var(--shadow-card)]" :alt="`${AUTHOR_NAME}的头像`" />
      <h2 id="profile-card-name" class="mt-4 font-[family-name:var(--font-family-display)] text-2xl font-bold text-text-primary">{{ AUTHOR_NAME }}</h2>
      <p class="mt-1 text-sm text-text-secondary">PengYuee · 个人博客</p>
    </div>
    <nav class="grid grid-cols-2 gap-2 p-4" aria-label="个人链接">
      <a v-for="link in PROFILE_LINKS" :key="link.id" :href="link.href" target="_blank" rel="noopener noreferrer" class="inline-flex min-h-11 items-center gap-2 rounded-lg px-3 text-sm text-text-secondary hover:bg-surface-hover hover:text-accent">
        <component :is="LINK_ICONS[link.id]" class="size-5 shrink-0" aria-hidden="true" />
        <span>{{ LINK_LABELS[link.id] }}</span>
      </a>
    </nav>
  </aside>
</template>