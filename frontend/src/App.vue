<script setup lang="ts">
import { RouterView } from "vue-router"
import AppToast from "@/components/shared/AppToast.vue"
import ThemeSwitcher from "@/components/shared/ThemeSwitcher.vue"
</script>

<template>
  <RouterView />
  <AppToast />
  <ThemeSwitcher />
</template>
