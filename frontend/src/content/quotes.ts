/** 本地名言条目。 */
export type LocalQuote = {
  /** 稳定标识。 */ readonly id: string
  /** 名言正文。 */ readonly text: string
  /** 出处。 */ readonly source: string
}

/** 默认名言，亦作为严格索引的安全回退。 */
const DEFAULT_QUOTE: LocalQuote = { id: "journey", text: "路虽远，行则将至；事虽难，做则必成。", source: "荀子" }

/** 随应用发布的本地名言库，不依赖网络服务。 */
export const LOCAL_QUOTES: readonly LocalQuote[] = [
  DEFAULT_QUOTE,
  { id: "learning", text: "学而不思则罔，思而不学则殆。", source: "论语" },
  { id: "practice", text: "纸上得来终觉浅，绝知此事要躬行。", source: "陆游" },
  { id: "mountain", text: "千里之行，始于足下。", source: "道德经" },
  { id: "clarity", text: "知者不惑，仁者不忧，勇者不惧。", source: "论语" },
] as const

/**
 * 选择一条本地名言，并在存在候选时避开上一条。
 * @param previousId 上一条名言标识。
 * @param random 可注入的随机数来源。
 * @returns 当前导航周期应展示的名言。
 */
export const pickLocalQuote = (previousId?: string, random: () => number = Math.random): LocalQuote => {
  const candidates = previousId === undefined ? LOCAL_QUOTES : LOCAL_QUOTES.filter((quote) => quote.id !== previousId)
  const index = Math.floor(random() * candidates.length)
  return candidates[index] ?? DEFAULT_QUOTE
}