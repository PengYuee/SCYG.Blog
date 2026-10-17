<script setup lang="ts">
import AiRunView from "@/components/ai/AiRunView.vue"
import PublicLayout from "@/layouts/PublicLayout.vue"
import "@/assets/ai.css"
</script>
<template><PublicLayout title="公共搜索任务"><AiRunView :anonymous="true" /></PublicLayout></template>
