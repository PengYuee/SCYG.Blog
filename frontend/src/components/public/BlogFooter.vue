<script setup lang="ts">
import { HeartIcon } from "@heroicons/vue/20/solid"
import { AUTHOR_NAME } from "@/content/profile"
</script>

<template>
  <footer class="border-t border-border-subtle bg-surface py-8 text-center text-sm text-text-secondary">
    <p class="inline-flex items-center gap-2">
      <span>2020 © qwfy · {{ AUTHOR_NAME }}</span>
      <HeartIcon class="size-4 text-accent" aria-hidden="true" />
    </p>
  </footer>
</template>