/** 个人站外链接标识。 */
export type ProfileLinkId = "qq" | "gitee" | "cnblogs" | "github"

/** 个人站外链接。 */
export type ProfileLink = {
  /** 链接标识。 */ readonly id: ProfileLinkId
  /** 可见名称。 */ readonly label: string
  /** 真实目标地址。 */ readonly href: string
}

/** 旧站确认的个人身份。 */
export const AUTHOR_NAME = "妄揽明月"

/** 旧站确认的个人链接，顺序保持不变。 */
export const PROFILE_LINKS: readonly ProfileLink[] = [
  { id: "qq", label: "QQ · 妄揽明月", href: "http://wpa.qq.com/msgrd?v=3&uin=798513422&site=qq&menu=yes" },
  { id: "gitee", label: "Gitee · 妄揽明月", href: "https://gitee.com/wlmy1996/personal_website" },
  { id: "cnblogs", label: "博客园 · 妄揽明月", href: "https://www.cnblogs.com/qwfy-y/" },
  { id: "github", label: "GitHub · PengYuee", href: "https://github.com/PengYuee" },
] as const