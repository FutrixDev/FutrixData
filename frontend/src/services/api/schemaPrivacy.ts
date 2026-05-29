import {
  SchemaPrivacyListConsents,
  SchemaPrivacyGetConsent,
  SchemaPrivacySetConsent,
  SchemaPrivacyListAudit,
} from '@wailsjs/go/main/App'

import { call, withMock } from './core'

export type SchemaConsent = '' | 'allowed' | 'denied'

export type SchemaConsentSummary = {
  datasourceId: string
  datasourceName: string
  datasourceType: string
  consent: SchemaConsent
  // RFC3339 string from the Go backend (schemaprivacy.AuditEntry.CreatedAt).
  // The frontend used to type this as a number and multiply by 1000, which
  // returned NaN for ISO strings — verify before changing back.
  lastSentAt?: string
  lastStatus?: string
}

export type SchemaAuditEntry = {
  id: string
  datasourceId: string
  datasourceName: string
  datasourceType: string
  triggerSource: string
  status: 'allowed' | 'denied'
  entityCount: number
  fieldCount: number
  includesComments: boolean
  providerType: string
  model: string
  aiConfigId: string
  reason: string
  createdAt: string
}

const unwrap = <T>(value: any, fallback: T): T => {
  if (!value || typeof value !== 'object') return fallback
  if (value.error) throw new Error(String(value.error))
  return value as T
}

export const schemaPrivacyApi = {
  listConsents: () =>
    withMock(
      async () => unwrap<{ items: SchemaConsentSummary[] }>(await SchemaPrivacyListConsents(), { items: [] }),
      async () => ({ items: [] as SchemaConsentSummary[] }),
    ),
  getConsent: (datasourceId: string) =>
    withMock(
      async () => unwrap<SchemaConsentSummary>(await SchemaPrivacyGetConsent(datasourceId), {
        datasourceId,
        datasourceName: '',
        datasourceType: '',
        consent: '' as SchemaConsent,
      }),
      async () => ({
        datasourceId,
        datasourceName: '',
        datasourceType: '',
        consent: '' as SchemaConsent,
      }),
    ),
  setConsent: (datasourceId: string, consent: SchemaConsent) =>
    withMock(
      async () => unwrap<{ datasourceId: string; consent: SchemaConsent }>(
        await call(() => SchemaPrivacySetConsent(datasourceId, consent)),
        { datasourceId, consent },
      ),
      async () => ({ datasourceId, consent }),
    ),
  listAudit: (datasourceId = '', limit = 50) =>
    withMock(
      async () => unwrap<{ items: SchemaAuditEntry[] }>(await SchemaPrivacyListAudit(datasourceId, limit), { items: [] }),
      async () => ({ items: [] as SchemaAuditEntry[] }),
    ),
}
