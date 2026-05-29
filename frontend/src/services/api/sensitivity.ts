import { call, cloneJson, withMock } from './core'
import { tApp, tAppEn } from '@/modules/i18n/appI18n'

const defaultLevelMeta = [
  { id: 1, key: 'L1', color: 'green' },
  { id: 2, key: 'L2', color: 'blue' },
  { id: 3, key: 'L3', color: 'yellow' },
  { id: 4, key: 'L4', color: 'orange' },
  { id: 5, key: 'L5', color: 'red' },
]

const levelNameKey = (key: string) => `sensitivity.levelDef.${key}.name`
const levelDescKey = (key: string) => `sensitivity.levelDef.${key}.desc`

const buildDefaultLevelConfig = () => ({
  levels: defaultLevelMeta.map((level) => ({
    ...level,
    name: tAppEn(levelNameKey(level.key)),
    nameEn: tAppEn(levelNameKey(level.key)),
    description: tAppEn(levelDescKey(level.key)),
    descriptionEn: tAppEn(levelDescKey(level.key)),
  })),
  agentAccessFrom: 1,
  agentAccessTo: 3,
})

const localizeLevelConfig = (config: ReturnType<typeof buildDefaultLevelConfig>) => {
  const cloned = cloneJson(config)
  cloned.levels = cloned.levels.map((level) => {
    const nameEn = level.nameEn || tAppEn(levelNameKey(level.key))
    const descriptionEn = level.descriptionEn || tAppEn(levelDescKey(level.key))
    return {
      ...level,
      nameEn,
      descriptionEn,
      name: level.name === nameEn ? tApp(levelNameKey(level.key)) : level.name,
      description: level.description === descriptionEn ? tApp(levelDescKey(level.key)) : level.description,
    }
  })
  return cloned
}

let mockCustomRules = ''
let mockMode = 'whitelist'
let mockLevelConfig = buildDefaultLevelConfig()

const mockProgress = (datasourceId: string) => ({
  datasourceId,
  status: 'completed',
  totalEntities: 0,
  scannedEntities: 0,
})

const parseLevels = (levelsJSON: string) => {
  try {
    const levels = JSON.parse(levelsJSON)
    return Array.isArray(levels) ? levels : []
  } catch {
    return []
  }
}

export const sensitivityApi = {
  scan: (datasourceId: string, aiConfigId: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.SensitivityScan(datasourceId, aiConfigId)),
      async () => ({ status: 'started', datasourceId, aiConfigId }),
    ),

  getProgress: (datasourceId: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.SensitivityGetProgress(datasourceId)),
      async () => mockProgress(datasourceId),
    ),

  getReport: (datasourceId: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.SensitivityGetReport(datasourceId)),
      async () => ({ found: false, datasourceId }),
    ),

  confirmField: (
    datasourceId: string,
    entityName: string,
    fieldName: string,
    level: string,
    category: string,
  ) =>
    withMock(
      () =>
        call(() =>
          (window as any).go.main.App.SensitivityConfirmField(
            datasourceId,
            entityName,
            fieldName,
            level,
            category,
          ),
        ),
      async () => ({ ok: true }),
    ),

  getMode: () =>
    withMock(
      () => call(() => (window as any).go.main.App.SensitivityGetMode()),
      async () => ({ mode: mockMode }),
    ),

  setMode: (mode: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.SensitivitySetMode(mode)),
      async () => {
        mockMode = mode
        return { ok: true }
      },
    ),

  getCustomRules: () =>
    withMock(
      () => call(() => (window as any).go.main.App.SensitivityGetCustomRules()),
      async () => ({ rules: mockCustomRules }),
    ),

  setCustomRules: (rules: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.SensitivitySetCustomRules(rules)),
      async () => {
        mockCustomRules = rules
        return { ok: true }
      },
    ),

  deleteDatasource: (datasourceId: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.SensitivityDeleteDatasource(datasourceId)),
      async () => ({ ok: true, datasourceId }),
    ),

  getLevelConfig: () =>
    withMock(
      () => call(() => (window as any).go.main.App.SensitivityGetLevelConfig()),
      async () => localizeLevelConfig(mockLevelConfig),
    ),

  setLevelConfig: (levelsJSON: string, agentAccessFrom: number, agentAccessTo: number) =>
    withMock(
      () => call(() => (window as any).go.main.App.SensitivitySetLevelConfig(levelsJSON, agentAccessFrom, agentAccessTo)),
      async () => {
        mockLevelConfig = {
          levels: parseLevels(levelsJSON),
          agentAccessFrom,
          agentAccessTo,
        }
        return { ok: true }
      },
    ),

  resetLevelConfig: () =>
    withMock(
      () => call(() => (window as any).go.main.App.SensitivityResetLevelConfig()),
      async () => {
        mockLevelConfig = buildDefaultLevelConfig()
        return { ok: true }
      },
    ),
}
