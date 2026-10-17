export type RequestAuth = "author" | "anonymous" | "none"
type Domain = Exclude<RequestAuth, "none">
const domains: Record<Domain, { token: () => string | undefined; invalidate: () => void }> = {
  author: { token: () => undefined, invalidate: () => undefined },
  anonymous: { token: () => undefined, invalidate: () => undefined },
}
export function configureRequestAuth(domain: Domain, token: () => string | undefined, invalidate: () => void): void {
  domains[domain] = { token, invalidate }
}
export function requestToken(domain: RequestAuth): string | undefined { return domain === "none" ? undefined : domains[domain].token() }
export function rejectRequestSession(domain: RequestAuth, token: string | undefined): void {
  if (domain !== "none" && token !== undefined && domains[domain].token() === token) domains[domain].invalidate()
}
