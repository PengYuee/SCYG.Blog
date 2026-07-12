/// <reference types="vite/client" />

/** 前端环境变量定义。 */
interface ImportMetaEnv {
  /** 非生产环境的 Fake 作者与 Fake Auth 联合开关。 */
  readonly VITE_FAKE_AUTHOR?: "true" | "false"
}

/** Vite 注入的只读元数据。 */
interface ImportMeta {
  /** 当前构建环境变量。 */
  readonly env: ImportMetaEnv
}
