<script setup lang="ts">
import { MdEditor } from "md-editor-v3"
import "md-editor-v3/lib/style.css"
import type { ImageLifecycle } from "@/services/image-lifecycle"

/** 富文本编辑器属性。 */
const props = defineProps<{ readonly modelValue: string; readonly images: ImageLifecycle }>()
/** Markdown 模型更新事件。 */
const emit = defineEmits<{ "update:modelValue": [value: string]; uploadFailure: [] }>()

/** 上传图片成功后才把远程地址交给编辑器插入。 */
async function uploadImages(files: File[], callback: (urls: string[]) => void): Promise<void> {
  const results = await Promise.allSettled(files.map((file) => props.images.upload(file)))
  const urls: string[] = []
  let uploadSucceeded = true
  for (const result of results) {
    if (result.status === "rejected") {
      uploadSucceeded = false
    } else {
      urls.push(result.value)
    }
  }
  if (!uploadSucceeded) {
    props.images.retainForCancel(urls)
    emit("uploadFailure")
    return
  }
  callback(urls)
}
</script>

<template><MdEditor :model-value="modelValue" language="zh-CN" preview-theme="github" :on-upload-img="uploadImages" @update:model-value="$emit('update:modelValue', $event)" /></template>
