export type UserKBCategoryScope = 'all' | 'datasource'

export type UserKBParseStatus = 'queued' | 'ok' | 'failed'

export type UserKBSummaryStatus =
  | 'queued'
  | 'ok'
  | 'failed'
  | 'needs_provider'
  | 'skipped'

export interface UserKBCategory {
  id: string
  name: string
  description?: string
  scope: UserKBCategoryScope
  datasourceIds?: string[]
  createdAt: number
  updatedAt: number
}

export interface UserKBFile {
  id: string
  categoryId: string
  originalName: string
  ext: string
  size: number
  uploadPath: string
  parsedPath: string
  parseStatus: UserKBParseStatus
  parseError?: string
  summaryStatus: UserKBSummaryStatus
  summaryError?: string
  note?: string
  aiSummary?: string
  keywords?: string[]
  createdAt: number
  updatedAt: number
}

export interface UserKBStoreState {
  version: number
  categories: UserKBCategory[]
  files: UserKBFile[]
}

export interface UserKBViewState {
  state: UserKBStoreState
  aiProviderReady: boolean
  aiProviderMessage?: string
}

export interface UserKBCategoryInput {
  name: string
  description?: string
  scope: UserKBCategoryScope
  datasourceIds?: string[]
}

export interface UserKBUploadFileInput {
  name: string
  base64: string
}
