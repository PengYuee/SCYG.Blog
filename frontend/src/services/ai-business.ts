import { z } from "zod"
const resourceId = z.number().int().positive().max(Number.MAX_SAFE_INTEGER)
const sources = z.array(resourceId)
/** These models describe only fields rendered by the UI, not the HTTP envelope. */
export const aiResultModels = {
  search: z.object({ conclusion: z.string(), articles: z.array(z.object({ article_id: resourceId, title: z.string(), visibility: z.enum(["published", "draft", "archived"]), evidence: z.string() })) }),
  write: z.object({ title: z.string(), outline: z.string(), markdown: z.string(), source_article_ids: sources }),
  polish: z.object({ content: z.string(), change_summary: z.string() }),
  chat: z.object({ response: z.string(), source_article_ids: sources }),
}
export const confirmationModel = z.object({ title: z.string(), outline: z.string() })
export const textInputModel = z.object({ title: z.string(), prompt: z.string(), min_length: z.number().int().min(1), max_length: z.number().int().positive().max(16000) })
export const selectionModel = z.object({ title: z.string(), prompt: z.string(), options: z.array(z.object({ article_id: resourceId, title: z.string(), digest: z.string(), status: z.string() })), min_selections: z.number().int().nonnegative(), max_selections: z.number().int().nonnegative() })
