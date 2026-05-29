import { AiChatApprove, AiChatCancelStream, AiChatTurn, AiChatTurnStream } from '@wailsjs/go/main/App'
import type { aichat } from '@wailsjs/go/models'

import { withMock } from './core'
import { newId } from './core'
import { loadMockState } from './mockState'

const pending = new Map<string, { kind: string; payload: any }>()
const lastResultByConversation = new Map<string, any>()

type TrustLevel = 'approval' | 'cautious' | 'trusted' | 'danger'

const normalizeTrustLevel = (value: unknown): TrustLevel => {
  if (value === 'approval' || value === 'cautious' || value === 'trusted' || value === 'danger') {
    return value
  }
  return 'cautious'
}

const canAutoExecuteAtTrust = (trust: TrustLevel, riskLevel: string) => {
  if (trust === 'approval') return false
  if (trust === 'danger') return true
  if (trust === 'trusted') return riskLevel === 'low' || riskLevel === 'medium'
  return riskLevel === 'low'
}

const parseElasticsearchRequestShape = (statement: string) => {
  const lines = String(statement || '').split('\n')
  const firstLine = lines.find((line) => line.trim())?.trim() || ''
  const [rawMethod = '', rawPath = ''] = firstLine.split(/\s+/, 2)
  const method = rawMethod.toUpperCase()
  if (!method || !rawPath) return null
  const path = rawPath.startsWith('/') ? rawPath : `/${rawPath}`
  const bodyStart = lines.findIndex((line) => line.trim())
  const body = bodyStart >= 0 ? lines.slice(bodyStart + 1).join('\n').trim() : ''
  return { method, path, body }
}

const elasticsearchPathIsSearch = (path: string) =>
  path === '/_search' || path.endsWith('/_search')

const elasticsearchQueryContainsBroadClause = (value: unknown): boolean => {
  if (Array.isArray(value)) return value.some((child) => elasticsearchQueryContainsBroadClause(child))
  if (!value || typeof value !== 'object') return false

  for (const [rawKey, child] of Object.entries(value as Record<string, unknown>)) {
    const key = rawKey.trim().toLowerCase()
    if (key === 'match_all' || key === 'wildcard' || key === 'regexp') return true
    if (elasticsearchQueryContainsBroadClause(child)) return true
  }

  return false
}

const elasticsearchStatementIsLowRisk = (statement: string) => {
  const parsed = parseElasticsearchRequestShape(statement)
  if (!parsed) return false
  if ((parsed.method === 'GET' || parsed.method === 'HEAD') && parsed.path.includes('/_doc/')) {
    return true
  }
  if (!elasticsearchPathIsSearch(parsed.path) || !parsed.body) return false
  try {
    const payload = JSON.parse(parsed.body)
    const query = payload?.query
    const size = Number(payload?.size || 0)
    if (!query || typeof query !== 'object') return false
    if (payload?.aggs || payload?.aggregations) return false
    if (elasticsearchQueryContainsBroadClause(query)) return false
    return size > 0 && size <= 100
  } catch {
    return false
  }
}

const mockTurn = async (payload: aichat.TurnRequest): Promise<aichat.TurnResponse> => {
  const last = String(payload.messages?.at(-1)?.content ?? '').toLowerCase()
  const wantsCreate = last.includes('create datasource') || last.includes('创建数据源') || last.includes('新增数据源')
  const wantsDelete = last.includes('delete datasource') || last.includes('删除数据源')
  const wantsEnter = last.includes('enter') || last.includes('open') || last.includes('进入') || last.includes('打开')
  const wantsVisualize = last.includes('visualize')
    || last.includes('visualisation')
    || last.includes('visualization')
    || last.includes('chart')
    || last.includes('plot')
    || last.includes('可视化')
    || last.includes('图表')
    || last.includes('画图')
  const wantsElastic = last.includes('elasticsearch') || last.includes('elastic') || String(payload.pageContext?.currentDatasourceType || '').toLowerCase().includes('elasticsearch')
  const wantsPlanMode =
    last.includes('plan this')
    || last.includes('plan mode')
    || last.includes('workflow')
    || last.includes('multi-step')
    || last.includes('多步骤')
    || last.includes('分步骤')
    || last.includes('计划执行')
  const wantsExecute =
    last.includes('执行')
    || last.includes('run')
    || last.includes('execute')
    || last.includes('查询')
    || last.includes('select ')
    || last.includes('find(')

  if (wantsPlanMode) {
    return {
      assistantMessage: '',
      agent: {
        mode: 'plan_executor',
        complexity: 'complex',
        reason: 'The request requires a staged workflow with checkpoints.',
        confidence: 0.86,
      },
      plan: {
        title: 'Execution Plan',
        summary: 'Break the task into safe steps and iterate with validation.',
        markdown: '1. Clarify target and constraints\\n2. Collect required context\\n3. Draft and verify actions\\n4. Execute and summarize outcome',
        steps: [
          { id: 'step_1', title: 'Clarify target', description: 'Confirm business goal and output format.', status: 'completed' },
          { id: 'step_2', title: 'Collect context', description: 'Inspect datasource, schema, and recent results.', status: 'in_progress' },
          { id: 'step_3', title: 'Execute safely', description: 'Run approved actions and check side effects.', status: 'pending' },
        ],
      },
    } as any
  }

  if (wantsCreate) {
    const approvalId = `appr_mock_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 7)}`
    const ds = wantsElastic
      ? {
          name: 'Mock ES',
          type: 'elasticsearch',
          host: '127.0.0.1',
          port: 9200,
          database: '',
          username: '',
          password: '',
        }
      : {
          name: 'Mock DS',
          type: 'mysql',
          host: '127.0.0.1',
          port: 3306,
          database: 'appdb',
          username: 'root',
          password: '',
        }
    pending.set(approvalId, { kind: 'create_datasource', payload: ds })
    return {
      assistantMessage: 'I can create a datasource for you. Please confirm.',
      approval: {
        id: approvalId,
        kind: 'create_datasource',
        summary: `Create datasource "${ds.name}" (${ds.type}) at ${ds.host}:${ds.port} database "${ds.database}"`,
        payload: ds,
      },
    }
  }

  if (wantsDelete) {
    const state = await loadMockState()
    const target = state.datasources.at(-1)
    if (!target) {
      return { assistantMessage: 'No datasource found to delete (mock).' }
    }
    const approvalId = `appr_mock_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 7)}`
    pending.set(approvalId, { kind: 'delete_datasource', payload: { datasourceId: target.id, name: target.name } })
    return {
      assistantMessage: `I can delete "${target.name}". Please confirm.`,
      approval: {
        id: approvalId,
        kind: 'delete_datasource',
        summary: `Delete datasource "${target.name}" (${target.id})`,
        payload: { datasourceId: target.id, name: target.name },
      },
    }
  }

  if (wantsEnter) {
    const state = await loadMockState()
    const lower = last.toLowerCase()
    const byName = state.datasources.find((ds) => {
      const name = String(ds.name || '').toLowerCase()
      return name && lower.includes(name)
    })
    const byType = !byName
      ? state.datasources.find((ds) => lower.includes('mysql') && String(ds.type || '').toLowerCase().includes('mysql'))
        || state.datasources.find((ds) => (lower.includes('mongo') || lower.includes('mongodb')) && String(ds.type || '').toLowerCase().includes('mongo'))
        || state.datasources.find((ds) => lower.includes('redis') && String(ds.type || '').toLowerCase().includes('redis'))
        || state.datasources.find((ds) => (lower.includes('elastic') || lower.includes('elasticsearch')) && String(ds.type || '').toLowerCase().includes('elastic'))
      : null
    const target = byName || byType || state.datasources[0]
    if (target?.id) {
      return {
        assistantMessage: `Opening \`${target.name}\` in Console...`,
        effects: { navigateTo: `/console/${target.id}` },
      }
    }
  }

  if (wantsVisualize) {
    const convoId = String(payload.conversationId || '')
    const stored = convoId ? lastResultByConversation.get(convoId) : null
    const result = stored?.result
    const rows = Array.isArray(result?.rows) ? result.rows : []
    const approvalId = `appr_mock_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 7)}`

    if (!stored || !rows.length) {
      return { assistantMessage: 'No recent query result available to visualize (mock). Run a query first.' }
    }

    const approxBytes = (() => {
      try { return JSON.stringify(rows).length } catch { return 0 }
    })()

    pending.set(approvalId, { kind: 'create_visualization', payload: { conversationId: convoId, question: last } })
    return {
      assistantMessage: 'I can generate a visualization from the last result. Please confirm.',
      approval: {
        id: approvalId,
        kind: 'create_visualization',
        summary: `Send ${rows.length} rows to AI to generate visualization (mock)`,
        payload: {
          datasourceId: String(stored.datasourceId || ''),
          datasourceType: String(stored.datasourceType || ''),
          database: String(stored.database || ''),
          rowCount: Number(result?.rowCount || rows.length),
          payloadRows: rows.length,
          truncated: false,
          approxBytes,
          capturedAt: new Date().toISOString(),
        },
      },
    }
  }

  if (wantsExecute) {
    const state = await loadMockState()
    const currentId = String(payload.pageContext?.currentDatasourceId ?? '')
    const currentName = String(state.datasources.find((ds) => ds.id === currentId)?.name ?? '')
    const dsId = currentId || state.datasources[0]?.id || ''
    const dsLabel = currentName || state.datasources.find((ds) => ds.id === dsId)?.name || dsId || 'datasource'
    const dsRecord = state.datasources.find((ds) => ds.id === dsId)
    const trustLevel = normalizeTrustLevel((dsRecord?.options as any)?.trustLevel)

    const currentDatasourceType = String(payload.pageContext?.currentDatasourceType || '').toLowerCase()
    const isMongo = last.includes('mongo') || last.includes('mongodb') || currentDatasourceType.includes('mongo')
    const isRedis = last.includes('redis') || currentDatasourceType.includes('redis')
    const isElastic = wantsElastic || currentDatasourceType.includes('elastic')
    const isChroma = last.includes('chromadb') || last.includes('chroma') || currentDatasourceType.includes('chromadb')
    const wantsRedisGet = isRedis && /\bget\b/.test(last)
    const wantsRedisFlushAll = isRedis && (last.includes('flushall') || last.includes('flush all'))
    const wantsRedisFlushDb = isRedis && last.includes('flushdb')
    const isNoIndex = last.includes('no index') || last.includes('无索引') || last.includes('不走索引')
    const isLarge = last.includes('>1000') || last.includes('大于1000') || last.includes('2000') || last.includes('large')
    const usesIndex = !isNoIndex
    const examined = isLarge ? 2000 : 100
    const isWrite = last.includes('delete')
      || last.includes('删除')
      || last.includes('删掉')
      || last.includes('insert')
      || last.includes('新增')
      || last.includes('add row')
      || last.includes('update')
      || last.includes('更新')

    let statement = isMongo
      ? `{"action":"find","collection":"files","filter":{},"options":{"sort":{"_id":1},"projection":{"_id":1,"size":1},"limit":100}}`
      : isRedis
        ? (wantsRedisFlushAll ? 'FLUSHALL' : wantsRedisFlushDb ? 'FLUSHDB' : wantsRedisGet ? 'GET key' : 'SCAN 0 MATCH * COUNT 100')
        : isElastic
          ? (last.includes('_search') || last.includes('search') || last.includes('查询')
              ? 'POST /futrixdata-demo-1/_search\n{"query":{"match_all":{}},"size":10}'
              : 'GET /_cat/indices?v')
          : isChroma
            ? (last.includes('query') || last.includes('search') || last.includes('查询')
                ? 'POST /collections/futrix_docs/query\n{"query_texts":["futrixdata"],"n_results":5,"include":["documents","metadatas"]}'
                : 'POST /collections/futrix_docs/get\n{"limit":50,"include":["documents","metadatas"]}')
          : isLarge
            ? 'SELECT * FROM table_name ORDER BY id ASC LIMIT 2000;'
            : 'SELECT * FROM table_name ORDER BY id ASC LIMIT 100;'

    if (!isMongo && !isRedis && !isChroma) {
      if (last.includes('drop')) {
        statement = 'DROP TABLE table_name;'
      } else if (last.includes('truncate')) {
        statement = 'TRUNCATE TABLE table_name;'
      } else if (last.includes('delete') || last.includes('删除') || last.includes('删掉')) {
        statement = 'DELETE FROM table_name WHERE id = 1;'
      } else if (last.includes('insert') || last.includes('新增') || last.includes('add row')) {
        statement = "INSERT INTO table_name (id, name) VALUES (1, 'Alice');"
      } else if (last.includes('update') || last.includes('更新')) {
        statement = "UPDATE table_name SET name = 'Alice' WHERE id = 1;"
      }
    }

    if (isMongo && (isLarge || isNoIndex)) {
      const pad = 'x'.repeat(1200)
      statement = `{"action":"find","collection":"files","filter":{},"options":{"sort":{"_id":1},"projection":{"_id":1,"size":1},"limit":100,"comment":"${pad}"}}`
    }

    const executionRisk = (() => {
      if (isRedis) {
        const cmd = statement.trim().split(/\s+/)[0]?.toUpperCase() || ''
        if (cmd === 'FLUSHALL' || cmd === 'FLUSHDB' || cmd === 'SHUTDOWN') return { level: 'high', reasons: [cmd] }
        if (cmd === 'DEL') return { level: 'medium', reasons: ['DEL'] }
        if (cmd === 'GET') return { level: 'low', reasons: [] }
        return { level: 'medium', reasons: ['SCAN'] }
      }
      if (isElastic) {
        const parsed = parseElasticsearchRequestShape(statement)
        const method = parsed?.method || ''
        if (method === 'DELETE') return { level: 'high', reasons: ['DELETE'] }
        if (method === 'PUT' || method === 'PATCH') return { level: 'medium', reasons: [method] }
        if (elasticsearchStatementIsLowRisk(statement)) return { level: 'low', reasons: [] }
        if (method === 'POST') return { level: 'medium', reasons: ['REQUEST_SCOPE'] }
        return { level: 'medium', reasons: ['REQUEST_SCOPE'] }
      }
      if (isChroma) {
        if (statement.startsWith('POST /collections/') && (statement.includes('/get') || statement.includes('/query'))) {
          return { level: 'low', reasons: [] }
        }
        return { level: 'medium', reasons: ['REQUEST_SCOPE'] }
      }
      const keyword = statement.trim().split(/\s+/)[0]?.toLowerCase() || ''
      if (keyword === 'drop' || keyword === 'truncate') return { level: 'high', reasons: ['destructive DDL'] }
      if (keyword === 'delete') return { level: 'medium', reasons: ['DELETE'] }
      if (keyword === 'insert' || keyword === 'replace') return { level: 'medium', reasons: ['INSERT/REPLACE'] }
      if (keyword === 'update') return { level: 'medium', reasons: ['UPDATE'] }
      if (!usesIndex || examined > 1000 || isWrite) {
        return { level: 'medium', reasons: !usesIndex ? ['NO_INDEX'] : examined > 1000 ? ['WIDE_SCAN'] : ['WRITE'] }
      }
      return { level: 'low', reasons: [] }
    })()
    const autoExec = canAutoExecuteAtTrust(trustLevel, executionRisk.level)

    if (autoExec) {
      const result = {
        columns: ['id', 'name'],
        rows: [
          { id: 1, name: 'Alice' },
          { id: 2, name: 'Bob' },
          { id: 3, name: 'Carol' },
        ],
        rowCount: 3,
        elapsedMs: 12,
        hasMore: false,
      }

      lastResultByConversation.set(String(payload.conversationId || ''), {
        datasourceId: dsId,
        datasourceType: isMongo ? 'mongodb' : isRedis ? 'redis' : isElastic ? 'elasticsearch' : isChroma ? 'chromadb' : 'mysql',
        database: payload.pageContext?.currentDatabase || '',
        statement,
        result,
      })

      return {
        assistantMessage: [
          `I ran this on \`${dsLabel}\`:`,
          '',
          isMongo ? '```json' : isRedis ? '```redis' : isElastic || isChroma ? '```text' : '```sql',
          statement,
          '```',
          '',
          '---',
          '',
          '### Execution check',
          '',
          usesIndex ? '- EXPLAIN: ✅ uses index (mock)' : '- EXPLAIN: ⚠️ no index detected (mock)',
          `- Examined: keys=${examined} docs=${examined}`,
          '',
          `Auto-executed because ${executionRisk.level} risk is enabled in preferences (mock).`,
          '',
          '### Execution result (mock)',
          '',
          '- Rows returned: 3',
          '- Columns: 2',
          '',
          '_Results are shown in the Console results panel._',
        ].join('\n'),
        effects: {
          consoleResult: {
            datasourceId: dsId,
            datasourceType: isMongo ? 'mongodb' : isRedis ? 'redis' : isElastic ? 'elasticsearch' : isChroma ? 'chromadb' : 'mysql',
            database: payload.pageContext?.currentDatabase || '',
            statement,
            result,
          },
        } as any,
      }
    }

    const approvalId = `appr_mock_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 7)}`
    const datasourceType = isMongo ? 'mongodb' : isRedis ? 'redis' : isElastic ? 'elasticsearch' : isChroma ? 'chromadb' : 'mysql'
    const approvalExplain = (isRedis || isElastic || isChroma)
      ? { explainSkipped: true }
      : {
        explain: {
          usesIndex,
          indexes: usesIndex ? ['idx_mock'] : [],
          stages: usesIndex ? ['IXSCAN'] : ['COLLSCAN'],
          totalKeysExamined: examined,
          totalDocsExamined: examined,
        },
      }

    const approvalPayload = {
      datasourceId: dsId,
      datasourceName: dsLabel,
      datasourceType,
      database: payload.pageContext?.currentDatabase || '',
      statement,
      risk: executionRisk,
      trustLevel,
      ...(approvalExplain as any),
    }

    pending.set(approvalId, { kind: 'execute_statement', payload: { ...approvalPayload, pageSize: 100 } })
    return {
      assistantMessage: [
        `I can run this on \`${dsLabel}\`:`,
        '',
        isMongo ? '```json' : isRedis ? '```redis' : isChroma ? '```text' : '```sql',
        statement,
        '```',
        '',
        '---',
        '',
        '### Execution check',
        '',
        usesIndex ? '- EXPLAIN: ✅ uses index (mock)' : '- EXPLAIN: ⚠️ no index detected (mock)',
        `- Examined: keys=${examined} docs=${examined}`,
        '',
        'Approve to execute, or Reject to cancel.',
      ].join('\n'),
      approval: {
        id: approvalId,
        kind: 'execute_statement',
        summary: `Execute statement on "${dsLabel}"`,
        payload: approvalPayload,
      },
    }
  }

  if (last.includes('redis')) {
    return {
      assistantMessage: [
        '```redis',
        'GET key',
        '```',
        '',
        '- (mock) Replace `key` with your key name.',
      ].join('\n'),
    }
  }
  if (last.includes('mongo')) {
    return {
      assistantMessage: [
        '```javascript',
        'db.collection.find({}).limit(10)',
        '```',
        '',
        '- (mock)',
      ].join('\n'),
    }
  }
  if (last.includes('sql')) {
    return {
      assistantMessage: [
        '```sql',
        'SELECT * FROM table_name LIMIT 10;',
        '```',
        '',
        '-- (mock)',
      ].join('\n'),
    }
  }
  return { assistantMessage: 'Mock response generated in dev mode.' }
}

const mockApprove = async (payload: aichat.ApproveRequest): Promise<aichat.TurnResponse> => {
  const decision = String(payload.decision || '').toLowerCase()
  const record = pending.get(payload.approvalId)
  pending.delete(payload.approvalId)

  if (!record) {
    return { assistantMessage: 'Approval not found (mock).' }
  }

  if (decision === 'reject') {
    return { assistantMessage: 'OK, cancelled.' }
  }

  const state = await loadMockState()
  if (record.kind === 'create_datasource') {
    state.datasources.push({ id: newId('ds'), ...record.payload })
    return { assistantMessage: 'Created datasource (mock).', effects: { datasourcesChanged: true } }
  }
  if (record.kind === 'delete_datasource') {
    const id = String(record.payload?.datasourceId ?? '')
    state.datasources = state.datasources.filter((item) => item.id !== id)
    return { assistantMessage: 'Deleted datasource (mock).', effects: { datasourcesChanged: true } }
  }
  if (record.kind === 'execute_statement') {
    const datasourceId = String(record.payload?.datasourceId ?? '')
    const database = String(record.payload?.database ?? '')
    const statement = String(record.payload?.statement ?? '')
    const result = {
      columns: ['id', 'name'],
      rows: [
        { id: 1, name: 'Alice' },
        { id: 2, name: 'Bob' },
        { id: 3, name: 'Carol' },
      ],
      rowCount: 3,
      elapsedMs: 12,
      hasMore: false,
    }
    lastResultByConversation.set(String(payload.conversationId || ''), {
      datasourceId,
      datasourceType: String(record.payload?.datasourceType ?? 'mysql'),
      database,
      statement,
      result,
    })
    return {
      assistantMessage: [
        '### Execution result (mock)',
        '',
        '- Rows returned: 3',
        '- Columns: 2',
        '',
        '_Results are shown in the Console results panel._',
      ].join('\n'),
      effects: {
        consoleResult: {
          datasourceId,
          datasourceType: String(record.payload?.datasourceType ?? 'mysql'),
          database,
          statement,
          result,
        },
      } as any,
    }
  }
  if (record.kind === 'create_visualization') {
    const convoId = String(record.payload?.conversationId || payload.conversationId || '')
    const question = String(record.payload?.question || '')
    const stored = convoId ? lastResultByConversation.get(convoId) : null
    const result = stored?.result
    const rows = Array.isArray(result?.rows) ? result.rows : []
    const columns = Array.isArray(result?.columns) ? result.columns : Object.keys(rows[0] || {})

    const wantsThree = question.includes('three') || question.includes('3d') || question.includes('3-d') || question.includes('三维') || question.includes('3维')
    if (wantsThree) {
      const points = rows.map((r: any, i: number) => ({
        x: Number(r?.id ?? i),
        y: i,
        z: typeof r?.name === 'string' ? r.name.length : 0,
        color: '#4f46e5',
        size: 0.1,
        label: typeof r?.name === 'string' ? r.name : String(i),
      }))

      return {
        assistantMessage: 'Visualization ready (mock).',
        effects: {
          navigateTo: '/visualization',
          visualization: {
            title: '3D scatter (mock)',
            renderer: 'three',
            spec: { type: 'scatter3d', points, axes: { x: 'id', y: 'index', z: 'name.length' }, background: '#0b1020' },
            datasourceId: String(stored?.datasourceId || ''),
            database: String(stored?.database || ''),
            statement: String(stored?.statement || ''),
            rowCount: Number(result?.rowCount || rows.length),
          },
        } as any,
      }
    }

    const pickNumeric = columns.find((c) => rows.some((r: any) => typeof r?.[c] === 'number'))
    const pickCategory = columns.find((c) => rows.some((r: any) => typeof r?.[c] === 'string'))
    const x = pickCategory || columns[0] || 'x'
    const y = pickNumeric || columns.find((c) => c !== x) || columns[0] || 'y'

    const spec = {
      $schema: 'https://vega.github.io/schema/vega-lite/v5.json',
      data: { values: rows },
      mark: { type: 'bar' },
      encoding: {
        x: { field: x, type: 'nominal', title: x },
        y: { field: y, type: 'quantitative', title: y },
        color: { field: x, type: 'nominal', title: x },
      },
    }

    return {
      assistantMessage: 'Visualization ready (mock).',
      effects: {
        navigateTo: '/visualization',
        visualization: {
          title: `Bar chart: ${y} by ${x} (mock)`,
          renderer: 'vega_lite',
          spec,
          datasourceId: String(stored?.datasourceId || ''),
          database: String(stored?.database || ''),
          statement: String(stored?.statement || ''),
          rowCount: Number(result?.rowCount || rows.length),
        },
      } as any,
    }
  }
  return { assistantMessage: 'OK (mock).' }
}

const mockTurnStream = async (_payload: aichat.TurnRequest): Promise<aichat.StreamStartResponse> => ({
  streamId: newId('stream'),
})

const mockCancelStream = async (_streamId: string): Promise<boolean> => true

export const aiChatApi = {
  aiChatTurn: (payload: aichat.TurnRequest) => withMock(() => AiChatTurn(payload), () => mockTurn(payload)),
  aiChatTurnStream: (payload: aichat.TurnRequest) => withMock(() => AiChatTurnStream(payload), () => mockTurnStream(payload)),
  aiChatCancelStream: (streamId: string) => withMock(() => AiChatCancelStream(streamId), () => mockCancelStream(streamId)),
  aiChatApprove: (payload: aichat.ApproveRequest) => withMock(() => AiChatApprove(payload), () => mockApprove(payload)),
}
