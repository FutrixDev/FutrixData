import type { UserKBCategoryInput, UserKBUploadFileInput, UserKBViewState } from '@/types/userkb'
import { tApp } from '@/modules/i18n/appI18n'

import { withMock } from './core'

const emptyState = (): UserKBViewState => ({
  state: { version: 1, categories: [], files: [] },
  aiProviderReady: false,
  aiProviderMessage: tApp('kb.providerMessage.runtimeUnavailable'),
})

const wails = () => (window as any)?.go?.main?.App

export const userKBApi = {
  userKBList: () => withMock(() => wails().UserKBList(), async () => emptyState()),

  userKBCreateCategory: (input: UserKBCategoryInput) =>
    withMock(() => wails().UserKBCreateCategory(input), async () => emptyState()),

  userKBUpdateCategory: (id: string, input: UserKBCategoryInput) =>
    withMock(() => wails().UserKBUpdateCategory(id, input), async () => emptyState()),

  userKBDeleteCategory: (id: string) =>
    withMock(() => wails().UserKBDeleteCategory(id), async () => emptyState()),

  userKBUploadFiles: (categoryId: string, files: UserKBUploadFileInput[]) =>
    withMock(() => wails().UserKBUploadFiles(categoryId, files), async () => emptyState()),

  userKBDeleteFile: (fileId: string) =>
    withMock(() => wails().UserKBDeleteFile(fileId), async () => emptyState()),
}
