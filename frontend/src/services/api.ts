import { aiApi } from './api/aiconfig'
import { aiChatApi } from './api/aichat'
import { authApi } from './api/auth'
import { consoleApi } from './api/console'
import { datasourcesApi } from './api/datasources'
import { embeddingApi } from './api/embedding'
import { historyApi } from './api/history'
import { logsApi } from './api/logs'
import { redisProtobufApi } from './api/redisProtobuf'
import { skillApi } from './api/skill'
import { startupRecoveryApi } from './api/startupRecovery'
import { updaterApi } from './api/updater'
import { userKBApi } from './api/userkb'

export const api = {
  ...datasourcesApi,
  ...authApi,
  ...consoleApi,
  ...historyApi,
  ...logsApi,
  ...startupRecoveryApi,
  ...aiApi,
  ...aiChatApi,
  ...embeddingApi,
  ...userKBApi,
  ...skillApi,
  ...updaterApi,
  ...redisProtobufApi,
}
