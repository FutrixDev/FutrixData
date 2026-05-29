import type { VueWrapper } from '@vue/test-utils'

export const getConsoleStatementInput = (wrapper: VueWrapper<any>) => {
  const legacyTextarea = wrapper.find('#statement-input')
  if (legacyTextarea.exists()) return legacyTextarea
  return wrapper.get('.console-monaco-editor__fallback')
}
