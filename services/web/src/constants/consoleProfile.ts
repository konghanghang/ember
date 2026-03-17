export const refreshConsoleProfileKey = Symbol('refreshConsoleProfile')

export type RefreshConsoleProfile = () => Promise<void>
