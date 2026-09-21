<script setup lang="ts">
import { MoonIcon, SunIcon } from "@heroicons/vue/24/outline"
import { computed } from "vue"
import { currentTheme, toggleTheme } from "@/theme/theme"

/** 描述按钮即将执行的主题切换，供可访问名称和原生提示共同使用。 */
const switchLabel = computed(() => (currentTheme.value === "night" ? "切换到蓝天主题" : "切换到夜空主题"))
</script>

<template>
  <button type="button" class="theme-switcher" data-motion="lift" :aria-label="switchLabel" :title="switchLabel" @click="toggleTheme">
    <SunIcon v-if="currentTheme === 'night'" class="size-6" aria-hidden="true" />
    <MoonIcon v-else class="size-6" aria-hidden="true" />
  </button>
</template>

<style scoped>
.theme-switcher {
  position: fixed;
  right: var(--space-6);
  bottom: var(--space-6);
  z-index: var(--layer-floating-control);
  display: inline-flex;
  width: var(--space-12);
  height: var(--space-12);
  align-items: center;
  justify-content: center;
  border: var(--border-width) solid var(--color-border);
  border-radius: var(--radius-round);
  background: var(--color-surface);
  box-shadow: var(--shadow-switcher);
  color: var(--color-accent);
}

.theme-switcher:hover {
  background: var(--color-surface-hover);
  color: var(--color-accent-hover);
  transform: translateY(var(--interaction-lift-control));
}

.theme-switcher:active {
  transform: scale(var(--interaction-scale-pressed));
}
</style>
