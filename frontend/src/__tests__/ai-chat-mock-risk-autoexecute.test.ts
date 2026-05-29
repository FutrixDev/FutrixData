import { describe, expect, it } from 'vitest'

import { aiChatApi } from '@/services/api/aichat'
import { datasourcesApi } from '@/services/api/datasources'

const setTrust = (id: string, trust: 'approval' | 'cautious' | 'trusted' | 'danger') =>
  datasourcesApi.setDatasourceTrustLevel(id, trust)

describe('ai chat mock risk auto execute', () => {
  it('does not auto execute a low-risk statement when the datasource is in approval mode', async () => {
    await setTrust('ds_mysql', 'approval')
    const response = await aiChatApi.aiChatTurn({
      conversationId: 'mock_low_approval',
      pageContext: { currentDatasourceId: 'ds_mysql' },
      messages: [{ role: 'user', content: 'run select * from table_name' }],
    } as any)

    expect(response.approval?.kind).toBe('execute_statement')
    expect((response.approval as any)?.payload?.trustLevel).toBe('approval')
    expect((response.effects as any)?.consoleResult).toBeFalsy()
  })

  it('auto executes a medium-risk statement when the datasource is trusted', async () => {
    await setTrust('ds_mysql', 'trusted')
    const response = await aiChatApi.aiChatTurn({
      conversationId: 'mock_medium_trusted',
      pageContext: { currentDatasourceId: 'ds_mysql' },
      messages: [{ role: 'user', content: 'run select * from table_name no index' }],
    } as any)

    expect((response.effects as any)?.consoleResult).toBeTruthy()
    expect(response.approval).toBeFalsy()
  })

  it('does NOT auto execute a high-risk statement in trusted mode', async () => {
    await setTrust('ds_mysql', 'trusted')
    const response = await aiChatApi.aiChatTurn({
      conversationId: 'mock_high_trusted',
      pageContext: { currentDatasourceId: 'ds_mysql' },
      messages: [{ role: 'user', content: 'run drop table users' }],
    } as any)

    expect(response.approval?.kind).toBe('execute_statement')
    expect((response.approval as any)?.payload?.risk?.level).toBe('high')
    expect((response.effects as any)?.consoleResult).toBeFalsy()
  })

  it('auto executes a high-risk statement when the datasource is in danger mode', async () => {
    await setTrust('ds_mysql', 'danger')
    const response = await aiChatApi.aiChatTurn({
      conversationId: 'mock_high_danger',
      pageContext: { currentDatasourceId: 'ds_mysql' },
      messages: [{ role: 'user', content: 'run drop table users' }],
    } as any)

    expect((response.effects as any)?.consoleResult).toBeTruthy()
    expect(response.approval).toBeFalsy()
  })

  it('treats redis single-key reads as low risk and auto executes them by default (cautious)', async () => {
    await setTrust('ds_mysql', 'cautious')
    const response = await aiChatApi.aiChatTurn({
      conversationId: 'mock_redis_low',
      pageContext: { currentDatasourceId: 'ds_mysql', currentDatasourceType: 'redis' },
      messages: [{ role: 'user', content: 'run redis get key' }],
    } as any)

    expect((response.effects as any)?.consoleResult?.statement).toBe('GET key')
    expect(response.approval).toBeFalsy()
  })

  it('auto executes a high-risk redis command when the datasource is in danger mode', async () => {
    await setTrust('ds_mysql', 'danger')
    const response = await aiChatApi.aiChatTurn({
      conversationId: 'mock_redis_high_danger',
      pageContext: { currentDatasourceId: 'ds_mysql', currentDatasourceType: 'redis' },
      messages: [{ role: 'user', content: 'run redis flushall' }],
    } as any)

    expect((response.effects as any)?.consoleResult?.statement).toBe('FLUSHALL')
    expect(response.approval).toBeFalsy()
  })

  it('keeps flushdb distinct from flushall in redis danger-mode mock execution', async () => {
    await setTrust('ds_mysql', 'danger')
    const response = await aiChatApi.aiChatTurn({
      conversationId: 'mock_redis_flushdb_danger',
      pageContext: { currentDatasourceId: 'ds_mysql', currentDatasourceType: 'redis' },
      messages: [{ role: 'user', content: 'run redis flushdb' }],
    } as any)

    expect((response.effects as any)?.consoleResult?.statement).toBe('FLUSHDB')
    expect(response.approval).toBeFalsy()
  })

  it('treats broad elasticsearch cat requests as medium risk and requires approval in cautious mode', async () => {
    await setTrust('ds_mysql', 'cautious')
    const response = await aiChatApi.aiChatTurn({
      conversationId: 'mock_es_cat',
      pageContext: { currentDatasourceId: 'ds_mysql', currentDatasourceType: 'elasticsearch' },
      messages: [{ role: 'user', content: 'run elastic cat indices' }],
    } as any)

    expect(response.approval?.kind).toBe('execute_statement')
    expect((response.approval as any)?.payload?.risk?.level).toBe('medium')
    expect((response.effects as any)?.consoleResult).toBeFalsy()
  })

  it('treats nested elasticsearch wildcard queries as medium risk and requires approval in cautious mode', async () => {
    await setTrust('ds_mysql', 'cautious')
    const response = await aiChatApi.aiChatTurn({
      conversationId: 'mock_es_nested_wildcard',
      pageContext: { currentDatasourceId: 'ds_mysql', currentDatasourceType: 'elasticsearch' },
      messages: [{ role: 'user', content: 'run elastic search wildcard nested' }],
    } as any)

    expect(response.approval?.kind).toBe('execute_statement')
    expect((response.approval as any)?.payload?.risk?.level).toBe('medium')
    expect((response.effects as any)?.consoleResult).toBeFalsy()
  })

  it('treats chromadb read requests as low risk and keeps the datasource type (cautious)', async () => {
    await setTrust('ds_mysql', 'cautious')
    const response = await aiChatApi.aiChatTurn({
      conversationId: 'mock_chroma_low',
      pageContext: { currentDatasourceId: 'ds_mysql', currentDatasourceType: 'chromadb' },
      messages: [{ role: 'user', content: 'run chromadb get docs' }],
    } as any)

    expect((response.effects as any)?.consoleResult?.statement).toContain('POST /collections/futrix_docs/get')
    expect((response.effects as any)?.consoleResult?.datasourceType).toBe('chromadb')
    expect(response.approval).toBeFalsy()
  })
})
