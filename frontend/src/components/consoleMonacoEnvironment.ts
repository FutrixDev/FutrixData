import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'
import JsonWorker from 'monaco-editor/esm/vs/language/json/json.worker?worker'
import CssWorker from 'monaco-editor/esm/vs/language/css/css.worker?worker'
import HtmlWorker from 'monaco-editor/esm/vs/language/html/html.worker?worker'
import TsWorker from 'monaco-editor/esm/vs/language/typescript/ts.worker?worker'

type MonacoWorkerConstructor = new () => Worker

type MonacoEnvironmentHost = typeof globalThis & {
  MonacoEnvironment?: {
    getWorker?: (moduleId: string, label: string) => Worker
  }
  __futrixMonacoEnvironmentReady?: boolean
}

export const monacoWorkerKindForLabel = (label: string) => {
  const normalized = String(label || '').toLowerCase()
  if (normalized === 'json') return 'json'
  if (normalized === 'css' || normalized === 'scss' || normalized === 'less') return 'css'
  if (normalized === 'html' || normalized === 'handlebars' || normalized === 'razor') return 'html'
  if (normalized === 'typescript' || normalized === 'javascript') return 'typescript'
  return 'editor'
}

const workerForKind = (kind: ReturnType<typeof monacoWorkerKindForLabel>): MonacoWorkerConstructor => {
  if (kind === 'json') return JsonWorker
  if (kind === 'css') return CssWorker
  if (kind === 'html') return HtmlWorker
  if (kind === 'typescript') return TsWorker
  return EditorWorker
}

export const ensureMonacoEnvironment = () => {
  const host = globalThis as MonacoEnvironmentHost
  if (host.__futrixMonacoEnvironmentReady) return
  host.MonacoEnvironment = {
    ...host.MonacoEnvironment,
    getWorker(_moduleId: string, label: string) {
      const WorkerCtor = workerForKind(monacoWorkerKindForLabel(label))
      return new WorkerCtor()
    },
  }
  host.__futrixMonacoEnvironmentReady = true
}
