import { call, withMock } from './core'

export type StartupRecoveryAction = 'retry' | 'update_app' | 'open_logs' | 'move_aside_and_restart'

export type StartupRecoveryError = {
  reason: string
  message: string
  dataPath?: string
  dataDir?: string
  retentionDir?: string
  formatVersion?: number
  writerAppVersion?: string
  minReaderAppVersion?: string
  actions?: StartupRecoveryAction[]
  details?: string
}

export type StartupRecoveryStatus = {
  state: 'initializing' | 'ready' | 'failed'
  error?: StartupRecoveryError | null
  movedAside?: {
    retentionDir?: string
  } | null
}

const mockReadyStatus = async (): Promise<StartupRecoveryStatus> => ({ state: 'ready' })
const mockNoop = async (): Promise<void> => {}

export const startupRecoveryApi = {
  startupRecoveryStatus: () =>
    withMock(
      () => call<StartupRecoveryStatus>(() => (window as any).go.main.App.StartupRecoveryStatus()),
      mockReadyStatus,
    ),
  startupRecoveryRetry: () =>
    withMock(
      () => call<StartupRecoveryStatus>(() => (window as any).go.main.App.StartupRecoveryRetry()),
      mockReadyStatus,
    ),
  startupRecoveryOpenLogs: () =>
    withMock(
      () => call<void>(() => (window as any).go.main.App.StartupRecoveryOpenLogs()),
      mockNoop,
    ),
  startupRecoveryOpenUpdatePage: () =>
    withMock(
      () => call<void>(() => (window as any).go.main.App.StartupRecoveryOpenUpdatePage()),
      mockNoop,
    ),
  startupRecoveryMoveAsideAndRestart: (confirmed: boolean) =>
    withMock(
      () => call<StartupRecoveryStatus>(() => (window as any).go.main.App.StartupRecoveryMoveAsideAndRestart(confirmed)),
      mockReadyStatus,
    ),
}
