<script setup lang="ts">
import AiWorkspace from "@/components/ai/AiWorkspace.vue"
import PublicLayout from "@/layouts/PublicLayout.vue"
import "@/assets/ai.css"
</script>
<template><PublicLayout title="公共 AI 搜索"><AiWorkspace :anonymous="true" /></PublicLayout></template>
