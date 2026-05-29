import { call } from './core'

export type DiagnosticsSettings = {
  datasourceTimingLogEnabled: boolean
}

export const logsApi = {
  exportLogs: () => call(() => (window as any).go.main.App.ExportLogs()),
  getDiagnosticsSettings: () =>
    call<DiagnosticsSettings>(() => (window as any).go.main.App.GetDiagnosticsSettings()),
  setDatasourceTimingLogEnabled: (enabled: boolean) =>
    call<DiagnosticsSettings>(() => (window as any).go.main.App.SetDatasourceTimingLogEnabled(enabled)),
  recordClientError: (kind: string, message: string, detail: string) =>
    call(() => (window as any).go.main.App.RecordClientError(kind, message, detail)),
}
