<script setup lang="ts">
import { ref } from "vue"
import { useRoute, useRouter } from "vue-router"
import PublicLayout from "@/layouts/PublicLayout.vue"
import { resolveAuthRedirect } from "@/router/guards"
import { useAuthSession } from "@/services/auth-session"

const route = useRoute()
const router = useRouter()
const authSession = useAuthSession()
const username = ref("")
const password = ref("")
const submitting = ref(false)
const errorMessage = ref<string | null>(authSession.currentState().kind === "expired" ? "登录会话已过期，请重新登录" : null)

/** 提交凭据并返回原受保护地址。 */
const submit = async (): Promise<void> => {
  if (submitting.value) return
  submitting.value = true
  errorMessage.value = null
  try {
    await authSession.login({ username: username.value, password: password.value })
    password.value = ""
    await router.replace(resolveAuthRedirect(route.query["redirect"]))
  } catch (error) {
    password.value = ""
    errorMessage.value = error instanceof Error && error.message.trim().length > 0 ? error.message : "登录失败，请稍后重试"
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <PublicLayout title="作者登录" content-id="login-content">
    <section class="mx-auto max-w-md rounded-[var(--radius-card)] border border-border-subtle bg-surface p-8 shadow-[var(--shadow-card)]" aria-labelledby="login-heading">
      <h2 id="login-heading" class="font-display text-2xl font-semibold">进入写作台</h2>
      <p class="mt-2 text-sm text-text-secondary">使用后端作者账号登录。访问令牌只保存在当前浏览器。</p>
      <form class="mt-8 space-y-5" @submit.prevent="submit">
        <label class="block text-sm font-medium" for="login-username">用户名</label>
        <input id="login-username" v-model="username" name="username" autocomplete="username" required maxlength="64" class="min-h-11 w-full rounded-lg border border-border bg-canvas px-3 focus:border-accent focus:outline-none focus:ring-2 focus:ring-accent/30" />
        <label class="block text-sm font-medium" for="login-password">密码</label>
        <input id="login-password" v-model="password" name="password" type="password" autocomplete="current-password" required maxlength="256" class="min-h-11 w-full rounded-lg border border-border bg-canvas px-3 focus:border-accent focus:outline-none focus:ring-2 focus:ring-accent/30" />
        <p v-if="errorMessage !== null" role="alert" class="rounded-lg border border-error bg-error-soft px-4 py-3 text-sm text-error">{{ errorMessage }}</p>
        <button type="submit" :disabled="submitting" class="min-h-11 w-full rounded-lg bg-accent px-5 font-semibold text-canvas disabled:cursor-not-allowed disabled:opacity-60">
          {{ submitting ? "正在登录…" : "登录" }}
        </button>
      </form>
    </section>
  </PublicLayout>
</template>
