import { inject, type InjectionKey } from 'vue'

export type ConsoleViewContext = {
  [key: string]: any
}

export const consoleViewContextKey: InjectionKey<ConsoleViewContext> = Symbol('ConsoleViewContext')

export function useConsoleViewContext(): ConsoleViewContext {
  const ctx = inject(consoleViewContextKey, null)
  if (!ctx) {
    throw new Error('ConsoleViewContext not provided')
  }
  return ctx
}
