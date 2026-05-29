import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DatasourceListView from '@/views/DatasourceListView.vue'
import { useAppStore } from '@/stores/app'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceListView endpoint copy', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
  })

  it('redacts sql uri credentials for endpoint display/copy and keeps existing fallbacks', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })

    const mongoUri = 'mongodb://user:pass@host1:27017,host2:27017/db?replicaSet=rs0'
    const mysqlUri = 'mysql://root:secret@db.example.com:3306/app'
    const mysqlUriRedacted = 'mysql://db.example.com:3306/app'
    const mysqlDsn = 'root:secret@tcp(db.example.com:3306)/app?parseTime=true'
    const mysqlDsnRedacted = 'tcp(db.example.com:3306)/app?parseTime=true'
    const mysqlDsnWithAtInPassword = 'root:p@ss@tcp(db.example.com:3306)/app?parseTime=true'
    const mysqlDsnWithAtInPasswordRedacted = 'tcp(db.example.com:3306)/app?parseTime=true'
    const mysqlDsnWithSlashInPassword = 'root:pa/ss@tcp(db.example.com:3306)/app?parseTime=true'
    const mysqlDsnWithSlashInPasswordRedacted = 'tcp(db.example.com:3306)/app?parseTime=true'
    const mysqlDsnWithParenInPassword = 'root:pa(ss@tcp(db.example.com:3306)/app?parseTime=true'
    const mysqlDsnWithParenInPasswordRedacted = 'tcp(db.example.com:3306)/app?parseTime=true'
    const mysqlDsnWithAtInQuery = 'root:secret@tcp(db.example.com:3306)/app?trace=user@example.com'
    const mysqlDsnWithAtInQueryRedacted = 'tcp(db.example.com:3306)/app?trace=user@example.com'
    const mysqlDsnWithoutDbSlash = 'root:secret@tcp(db.example.com:3306)'
    const mysqlDsnWithoutDbSlashRedacted = 'tcp(db.example.com:3306)'
    const pgUri = 'postgresql://postgres:secret@db.example.com:5432/postgres'
    const pgUriRedacted = 'postgresql://db.example.com:5432/postgres'
    const malformedPgUri = 'postgresql://postgres:secret@/postgres'
    const malformedPgUriRedacted = 'postgresql:///postgres'
    const pgUriWithQueryAuth = 'postgresql://db.example.com:5432/postgres?user=alice&password=secret&sslmode=require'
    const pgKeywordDsn = 'host=db.example.com port=5432 user=postgres password=secret dbname=postgres sslmode=require'
    const pgKeywordDsnRedacted = 'host=db.example.com port=5432 user=*** password=*** dbname=postgres sslmode=require'
    const pgKeywordDsnWithQuotes = `host=db.example.com user='alice' password="secret value" dbname=postgres`
    const pgKeywordDsnWithQuotesRedacted = `host=db.example.com user='***' password="***" dbname=postgres`
    const dynamoEndpoint = 'http://127.0.0.1:8000'
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_mongo',
        name: 'A Mongo',
        type: 'mongodb',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: 'db',
        authSource: '',
        options: { uri: mongoUri },
      },
      {
        id: 'ds_mysql',
        name: 'B MySQL',
        type: 'mysql',
        host: '127.0.0.1',
        port: 3306,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {},
      },
      {
        id: 'ds_mysql_non_string_uri',
        name: 'MySQL Non-String URI',
        type: 'mysql',
        host: '10.1.1.9',
        port: 3307,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          uri: 123 as any,
        },
      },
      {
        id: 'ds_mysql_uri',
        name: 'MySQL URI',
        type: 'mysql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          uri: mysqlUri,
        },
      },
      {
        id: 'ds_mysql_dsn',
        name: 'MySQL DSN',
        type: 'mysql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          uri: mysqlDsn,
        },
      },
      {
        id: 'ds_mysql_dsn_at',
        name: 'MySQL DSN @ Password',
        type: 'mysql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          uri: mysqlDsnWithAtInPassword,
        },
      },
      {
        id: 'ds_mysql_dsn_slash',
        name: 'MySQL DSN / Password',
        type: 'mysql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          uri: mysqlDsnWithSlashInPassword,
        },
      },
      {
        id: 'ds_mysql_dsn_query_at',
        name: 'MySQL DSN Query @',
        type: 'mysql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          uri: mysqlDsnWithAtInQuery,
        },
      },
      {
        id: 'ds_mysql_dsn_paren',
        name: 'MySQL DSN ( Password',
        type: 'mysql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          uri: mysqlDsnWithParenInPassword,
        },
      },
      {
        id: 'ds_mysql_dsn_no_slash',
        name: 'MySQL DSN No Slash',
        type: 'mysql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          uri: mysqlDsnWithoutDbSlash,
        },
      },
      {
        id: 'ds_pg_uri',
        name: 'PG URI',
        type: 'postgresql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          uri: pgUri,
        },
      },
      {
        id: 'ds_pg_malformed',
        name: 'PG Malformed URI',
        type: 'postgresql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          uri: malformedPgUri,
        },
      },
      {
        id: 'ds_pg_query_auth',
        name: 'PG Query Auth URI',
        type: 'postgresql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          uri: pgUriWithQueryAuth,
        },
      },
      {
        id: 'ds_pg_keyword_dsn',
        name: 'PG Keyword DSN',
        type: 'postgresql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          uri: pgKeywordDsn,
        },
      },
      {
        id: 'ds_pg_keyword_dsn_quotes',
        name: 'PG Keyword DSN Quotes',
        type: 'postgresql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          uri: pgKeywordDsnWithQuotes,
        },
      },
      {
        id: 'ds_ddb',
        name: 'C DynamoDB',
        type: 'dynamodb',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: { region: 'us-east-1', endpoint: dynamoEndpoint },
      },
    ]

    const wrapper = mount(DatasourceListView, {
      global: {
        plugins: [pinia],
      },
    })

    const copyButtons = wrapper.findAll('[data-testid="datasource-endpoint-copy"]')
    expect(copyButtons).toHaveLength(16)
    const firstButton = copyButtons[0]
    expect(firstButton.attributes('aria-label')).toBe('Copy endpoint')
    expect(firstButton.find('svg').exists()).toBe(true)

    const cardByName = (name: string) => wrapper.findAll('.datasource-card').find((card) => card.text().includes(name))
    const mongoCard = cardByName('A Mongo')
    const mysqlCard = cardByName('B MySQL')
    const mysqlNonStringUriCard = cardByName('MySQL Non-String URI')
    const mysqlUriCard = cardByName('MySQL URI')
    const mysqlDsnCard = cardByName('MySQL DSN')
    const mysqlDsnAtCard = cardByName('MySQL DSN @ Password')
    const mysqlDsnSlashCard = cardByName('MySQL DSN / Password')
    const mysqlDsnQueryAtCard = cardByName('MySQL DSN Query @')
    const mysqlDsnParenCard = cardByName('MySQL DSN ( Password')
    const mysqlDsnNoSlashCard = cardByName('MySQL DSN No Slash')
    const pgUriCard = cardByName('PG URI')
    const malformedPgUriCard = cardByName('PG Malformed URI')
    const pgQueryAuthCard = cardByName('PG Query Auth URI')
    const pgKeywordDsnCard = cardByName('PG Keyword DSN')
    const pgKeywordDsnQuotesCard = cardByName('PG Keyword DSN Quotes')
    const ddbCard = cardByName('C DynamoDB')

    expect(mongoCard).toBeTruthy()
    expect(mysqlCard).toBeTruthy()
    expect(mysqlNonStringUriCard).toBeTruthy()
    expect(mysqlUriCard).toBeTruthy()
    expect(mysqlDsnCard).toBeTruthy()
    expect(mysqlDsnAtCard).toBeTruthy()
    expect(mysqlDsnSlashCard).toBeTruthy()
    expect(mysqlDsnQueryAtCard).toBeTruthy()
    expect(mysqlDsnParenCard).toBeTruthy()
    expect(mysqlDsnNoSlashCard).toBeTruthy()
    expect(pgUriCard).toBeTruthy()
    expect(malformedPgUriCard).toBeTruthy()
    expect(pgQueryAuthCard).toBeTruthy()
    expect(pgKeywordDsnCard).toBeTruthy()
    expect(pgKeywordDsnQuotesCard).toBeTruthy()
    expect(ddbCard).toBeTruthy()

    await mongoCard!.find('[data-testid="datasource-endpoint-copy"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith(mongoUri)

    await mysqlCard!.find('[data-testid="datasource-endpoint-copy"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith('127.0.0.1:3306')

    await mysqlNonStringUriCard!.find('[data-testid="datasource-endpoint-copy"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith('10.1.1.9:3307')

    await mysqlUriCard!.find('[data-testid="datasource-endpoint-copy"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith(mysqlUriRedacted)

    await mysqlDsnCard!.find('[data-testid="datasource-endpoint-copy"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith(mysqlDsnRedacted)

    await mysqlDsnAtCard!.find('[data-testid="datasource-endpoint-copy"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith(mysqlDsnWithAtInPasswordRedacted)

    await mysqlDsnSlashCard!.find('[data-testid="datasource-endpoint-copy"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith(mysqlDsnWithSlashInPasswordRedacted)

    await mysqlDsnQueryAtCard!.find('[data-testid="datasource-endpoint-copy"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith(mysqlDsnWithAtInQueryRedacted)

    await mysqlDsnParenCard!.find('[data-testid="datasource-endpoint-copy"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith(mysqlDsnWithParenInPasswordRedacted)

    await mysqlDsnNoSlashCard!.find('[data-testid="datasource-endpoint-copy"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith(mysqlDsnWithoutDbSlashRedacted)

    await pgUriCard!.find('[data-testid="datasource-endpoint-copy"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith(pgUriRedacted)

    await malformedPgUriCard!.find('[data-testid="datasource-endpoint-copy"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith(malformedPgUriRedacted)

    await pgQueryAuthCard!.find('[data-testid="datasource-endpoint-copy"]').trigger('click')
    const pgQueryAuthCopied = writeText.mock.calls[writeText.mock.calls.length - 1]?.[0] as string
    expect(pgQueryAuthCopied).toContain('user=***')
    expect(pgQueryAuthCopied).toContain('password=***')
    expect(pgQueryAuthCopied).not.toContain('user=alice')
    expect(pgQueryAuthCopied).not.toContain('password=secret')

    await pgKeywordDsnCard!.find('[data-testid="datasource-endpoint-copy"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith(pgKeywordDsnRedacted)

    await pgKeywordDsnQuotesCard!.find('[data-testid="datasource-endpoint-copy"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith(pgKeywordDsnWithQuotesRedacted)

    await ddbCard!.find('[data-testid="datasource-endpoint-copy"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith(dynamoEndpoint)

    expect(wrapper.text()).toContain(mysqlUriRedacted)
    expect(wrapper.text()).toContain(mysqlDsnRedacted)
    expect(wrapper.text()).toContain(mysqlDsnWithAtInPasswordRedacted)
    expect(wrapper.text()).toContain(mysqlDsnWithSlashInPasswordRedacted)
    expect(wrapper.text()).toContain(mysqlDsnWithAtInQueryRedacted)
    expect(wrapper.text()).toContain(mysqlDsnWithParenInPasswordRedacted)
    expect(wrapper.text()).toContain(mysqlDsnWithoutDbSlashRedacted)
    expect(wrapper.text()).toContain(pgUriRedacted)
    expect(wrapper.text()).toContain(malformedPgUriRedacted)
    expect(wrapper.text()).toContain('user=***')
    expect(wrapper.text()).toContain('password=***')
    expect(wrapper.text()).toContain(pgKeywordDsnRedacted)
    expect(wrapper.text()).toContain(pgKeywordDsnWithQuotesRedacted)
    expect(wrapper.text()).not.toContain('root:secret@')
    expect(wrapper.text()).not.toContain('root:p@ss@')
    expect(wrapper.text()).not.toContain('root:pa/ss@')
    expect(wrapper.text()).not.toContain('root:pa(ss@')
    expect(wrapper.text()).not.toContain('root:secret@tcp')
    expect(wrapper.text()).not.toContain('root:secret@tcp(db.example.com:3306)')
    expect(wrapper.text()).not.toContain('postgres:secret@')
    expect(wrapper.text()).not.toContain('user=alice')
    expect(wrapper.text()).not.toContain('password=secret')
    expect(wrapper.text()).not.toContain("user='alice'")
    expect(wrapper.text()).not.toContain('password="secret value"')
  })
})
