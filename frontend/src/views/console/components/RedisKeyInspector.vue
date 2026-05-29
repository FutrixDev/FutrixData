<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { tApp } from '@/modules/i18n/appI18n'
import { useConsoleViewContext } from '../context'
import RedisTypeBadge from '@/components/redis/RedisTypeBadge.vue'
import { redisTypeAccent } from '@/modules/redis/type-theme'

type InspectorTab = 'preview' | 'output'

const ctx = useConsoleViewContext()

const store = ctx.store
const isRedis = ctx.isRedis

const startRedisNewKey = ctx.startRedisNewKey
const copyRedisKey = ctx.copyRedisKey

const redisDetailItems = ctx.redisDetailItems
const redisPreview = ctx.redisPreview

const redisFullLoading = ctx.redisFullLoading
const redisFullValue = ctx.redisFullValue
const redisFullView = (ctx as any).redisFullView
const redisFullError = ctx.redisFullError
const loadRedisFullPreview = ctx.loadRedisFullPreview

const previewKind = computed(() => String(redisPreview.value?.kind || ''))
const previewAccent = computed(() => redisTypeAccent(previewKind.value))

const activeView = computed(() => {
  if (redisFullView?.value) return redisFullView.value
  const p = redisPreview.value
  if (!p) return null
  return {
    kind: p.kind,
    headers: p.headers,
    rows: p.rows,
    isEmpty: !p.rows?.length && (p.kind !== 'string' || !p.rows?.[0]?.[0]),
  }
})

const isShowingFull = computed(() => Boolean(redisFullView?.value))

const emptyCopyKey = computed(() => {
  const k = previewKind.value || 'string'
  switch (k) {
    case 'hash':
      return 'redis.inspector.emptyHash'
    case 'list':
      return 'redis.inspector.emptyList'
    case 'set':
      return 'redis.inspector.emptySet'
    case 'zset':
      return 'redis.inspector.emptyZset'
    case 'stream':
      return 'redis.inspector.emptyStream'
    case 'string':
    default:
      return 'redis.inspector.emptyString'
  }
})

const typeLabelLong = computed(() => {
  const k = previewKind.value || ''
  if (!k) return ''
  return tApp(`redis.inspector.typeLong.${k}`)
})

const stringPreviewRaw = computed<string>(() => {
  if (previewKind.value !== 'string') return ''
  if (redisFullView?.value && redisFullView.value.kind === 'string') {
    return String(redisFullView.value.rows?.[0]?.[0] ?? '')
  }
  return String(redisPreview.value?.rows?.[0]?.[0] ?? '')
})

// Full-string fetch uses GET, which returns the entire value. Eagerly
// JSON.parse-ing arbitrarily large payloads can freeze the UI thread for many
// seconds. Gate the auto-pretty path behind a size cap; oversize strings still
// render as raw text (no chip, no toggle), and the parse is deferred until the
// user explicitly clicks "Pretty".
const JSON_PRETTY_MAX_BYTES = 256 * 1024

const isStringMaybeJson = computed(() => {
  const raw = stringPreviewRaw.value
  if (!raw) return false
  if (raw.length > JSON_PRETTY_MAX_BYTES) return false
  const trimmed = raw.trim()
  if (!trimmed) return false
  return trimmed[0] === '{' || trimmed[0] === '['
})

const showPrettyJson = ref(false)

const parsedJsonPretty = computed<string | null>(() => {
  if (!showPrettyJson.value) return null
  if (!isStringMaybeJson.value) return null
  const raw = stringPreviewRaw.value
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw.trim())
    if (parsed === null || typeof parsed !== 'object') return null
    return JSON.stringify(parsed, null, 2)
  } catch {
    return null
  }
})

const isStringJson = computed(() => isStringMaybeJson.value)

// Binary detection. Backend sets `binary: true` + `valueB64` when the raw
// bytes can't be safely encoded as UTF-8 text (bitmaps, protobuf payloads,
// etc.). For the "View full" path the backend doesn't tag binary, so we also
// scan the rendered string for replacement chars and NUL/control bytes. We
// keep using the preview's b64 even when "View full" is active so the hex
// view stays lossless for binary; full text view is reserved for text.
type StringPreviewSource = { binary?: boolean; valueB64?: string; valueB64Truncated?: boolean }
const stringPreviewSource = computed<StringPreviewSource | null>(() => {
  if (previewKind.value !== 'string') return null
  const preview = (redisPreview.value as unknown as StringPreviewSource) || null
  if (preview?.binary) return preview
  if (redisFullView?.value && redisFullView.value.kind === 'string') {
    return null
  }
  return preview
})

const stringHeuristicBinary = computed(() => {
  const raw = stringPreviewRaw.value
  if (!raw) return false
  let suspicious = 0
  let scanned = 0
  const cap = Math.min(raw.length, 4096)
  for (let i = 0; i < cap; i++) {
    const code = raw.charCodeAt(i)
    scanned++
    if (code === 0xfffd) {
      suspicious++
    } else if (code < 0x20 && code !== 0x09 && code !== 0x0a && code !== 0x0d) {
      suspicious++
    } else if (code === 0x7f) {
      suspicious++
    }
  }
  if (scanned === 0) return false
  if (suspicious >= 4) return true
  return suspicious / scanned > 0.02
})

const stringIsBinary = computed(() => {
  if (stringPreviewSource.value?.binary) return true
  return stringHeuristicBinary.value
})

type StringViewMode = 'auto' | 'text' | 'hex'
const stringViewMode = ref<StringViewMode>('auto')

const effectiveStringViewMode = computed<'text' | 'hex'>(() => {
  if (stringViewMode.value === 'hex') return 'hex'
  if (stringViewMode.value === 'text') return 'text'
  return stringIsBinary.value ? 'hex' : 'text'
})

const stringHexBytes = computed<Uint8Array | null>(() => {
  // Prefer raw bytes from the backend (lossless) when present.
  const b64 = stringPreviewSource.value?.valueB64
  if (b64) {
    try {
      const bin = atob(b64)
      const out = new Uint8Array(bin.length)
      for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
      return out
    } catch {
      // fall through to UTF-8 encoding of the (possibly lossy) string
    }
  }
  const raw = stringPreviewRaw.value
  if (!raw) return null
  try {
    return new TextEncoder().encode(raw)
  } catch {
    return null
  }
})

const stringHexDump = computed(() => {
  const bytes = stringHexBytes.value
  if (!bytes || bytes.length === 0) return ''
  const lines: string[] = []
  // Render the full payload the backend supplied. Backend already caps the
  // base64 prefix (64 KiB by default) and surfaces a separate truncation
  // notice, so further frontend trimming would hide bytes the user can
  // already see in the buffer.
  const cap = bytes.length
  for (let offset = 0; offset < cap; offset += 16) {
    const slice = bytes.subarray(offset, Math.min(offset + 16, cap))
    const hex: string[] = []
    let ascii = ''
    for (let i = 0; i < 16; i++) {
      if (i < slice.length) {
        const b = slice[i]
        hex.push(b.toString(16).padStart(2, '0'))
        ascii += b >= 0x20 && b < 0x7f ? String.fromCharCode(b) : '.'
      } else {
        hex.push('  ')
        ascii += ' '
      }
      if (i === 7) hex.push('')
    }
    lines.push(
      `${offset.toString(16).padStart(8, '0')}  ${hex.join(' ')}  ${ascii}`,
    )
  }
  return lines.join('\n')
})

const formatCell = ctx.formatCell

const result = ctx.result
const statusMessage = ctx.statusMessage
const statusType = ctx.statusType
const resultMeta = ctx.resultMeta
const resultRows = ctx.resultRows
const redisResultText = ctx.redisResultText
const copyRedisResults = ctx.copyRedisResults
const hasMultiResults = ctx.hasMultiResults
const multiResults = ctx.multiResults
const activeMultiResultId = ctx.activeMultiResultId
const selectMultiResult = ctx.selectMultiResult
const clearMultiResults = ctx.clearMultiResults

const showRedisCommandResult = computed(() => {
  if (!isRedis.value) return false
  return Boolean(statusMessage.value) || Boolean(resultMeta.value) || resultRows.value.length > 0
})

const canPreview = computed(() => Boolean(store.selectedEntity))
const canOutput = computed(() => showRedisCommandResult.value)

const activeInspectorTab = ref<InspectorTab>('preview')

const ensureInspectorTabValid = () => {
  if (activeInspectorTab.value === 'preview' && !canPreview.value && canOutput.value) {
    activeInspectorTab.value = 'output'
    return
  }
  if (activeInspectorTab.value === 'output' && !canOutput.value) {
    activeInspectorTab.value = canPreview.value ? 'preview' : 'preview'
  }
}

watch(
  () => store.selectedEntity,
  (next, prev) => {
    if (next && next !== prev) {
      activeInspectorTab.value = 'preview'
    }
    ensureInspectorTabValid()
  },
)

watch(canOutput, (value) => {
  if (value) {
    activeInspectorTab.value = 'output'
  } else {
    ensureInspectorTabValid()
  }
})

watch([canPreview, canOutput], ensureInspectorTabValid, { immediate: true })

const setInspectorTab = (tab: InspectorTab) => {
  if (tab === 'preview' && !canPreview.value) return
  if (tab === 'output' && !canOutput.value) return
  activeInspectorTab.value = tab
}

const clearRedisCommandResult = () => {
  clearMultiResults()
  result.value = null
  resultMeta.value = ''
  statusMessage.value = ''
  statusType.value = ''
}

const showStatusChip = computed(() => {
  const message = String(statusMessage.value || '').trim()
  if (!message) return false
  if (statusType.value === 'success' && message === 'Success') return false
  return true
})
</script>

<template>
  <div v-if="isRedis" class="redis-detail-card">
    <div class="redis-detail-head">
      <div>
        <div class="redis-detail-title">{{ tApp('redis.inspector.title') }}</div>
        <p class="meta">
          {{
            store.selectedEntity
              ? tApp('redis.inspector.meta.keyDetailsPreview')
              : showRedisCommandResult
                ? tApp('redis.inspector.meta.commandOutput')
                : tApp('redis.inspector.meta.selectKeyToInspect')
          }}
        </p>
      </div>
      <div class="redis-detail-actions">
        <button class="btn ghost mini" type="button" @click="startRedisNewKey">{{ tApp('redis.inspector.newKey') }}</button>
        <button
          v-if="store.selectedEntity"
          class="btn ghost mini"
          type="button"
          @click="copyRedisKey"
        >
          {{ tApp('redis.inspector.copyKey') }}
        </button>
      </div>
    </div>
    <div v-if="store.selectedEntity || showRedisCommandResult" class="redis-detail-body">
      <template v-if="store.selectedEntity">
        <div class="redis-key-row">
          <span class="redis-key-label">{{ tApp('redis.inspector.key') }}</span>
          <span class="redis-key-value">{{ store.selectedEntity }}</span>
        </div>
        <div class="redis-key-meta" v-if="redisDetailItems.length">
          <span v-for="item in redisDetailItems" :key="item.label" class="pill redis-pill">
            {{ item.label }}: {{ formatCell(item.value) }}
          </span>
        </div>
      </template>

      <div class="redis-inspector">
        <div class="redis-inspector-tabs" role="tablist" :aria-label="tApp('redis.inspector.tabs')">
          <button
            class="redis-inspector-tab"
            type="button"
            role="tab"
            data-testid="redis-inspector-tab-preview"
            :aria-selected="activeInspectorTab === 'preview'"
            :aria-disabled="!canPreview"
            :class="{ active: activeInspectorTab === 'preview', disabled: !canPreview }"
            @click="setInspectorTab('preview')"
          >
            {{ tApp('redis.inspector.previewTab') }}
          </button>
          <button
            class="redis-inspector-tab"
            type="button"
            role="tab"
            data-testid="redis-inspector-tab-output"
            :aria-selected="activeInspectorTab === 'output'"
            :aria-disabled="!canOutput"
            :class="{ active: activeInspectorTab === 'output', disabled: !canOutput }"
            @click="setInspectorTab('output')"
          >
            <span class="redis-inspector-dot" :class="canOutput ? (statusType || 'success') : ''"></span>
            {{ tApp('redis.inspector.outputTab') }}
          </button>
          <button
            v-if="canOutput"
            class="btn ghost mini redis-inspector-clear"
            type="button"
            @click="clearRedisCommandResult"
          >
            {{ tApp('redis.inspector.clearOutput') }}
          </button>
        </div>

        <div v-if="activeInspectorTab === 'preview'" class="redis-inspector-pane" data-testid="redis-inspector-preview">
          <div v-if="redisPreview" class="redis-preview" :data-redis-kind="previewKind">
            <div class="redis-preview-head">
              <span class="redis-preview-head-label">
                <RedisTypeBadge v-if="previewKind" :type="previewKind" />
                <span>{{ typeLabelLong || tApp('redis.inspector.previewTab') }}</span>
              </span>
              <div class="redis-preview-actions">
                <span v-if="isShowingFull" class="meta" data-testid="redis-preview-banner-all">
                  {{ tApp('redis.inspector.showingAll', { count: (redisFullView?.rows?.length || 0) }) }}
                </span>
                <span v-else class="meta">
                  {{ tApp('redis.inspector.firstItems', { limit: redisPreview.limit, kind: redisPreview.kind }) }}
                </span>
                <button
                  v-if="redisPreview.truncated && !redisFullLoading && !isShowingFull"
                  class="btn ghost mini"
                  type="button"
                  @click="loadRedisFullPreview"
                >
                  {{ tApp('redis.inspector.viewFull') }}
                </button>
              </div>
            </div>
            <div class="redis-preview-body">
              <div v-if="redisFullLoading" class="meta">{{ tApp('redis.inspector.loadingFull') }}</div>
              <template v-else>
                <div v-if="redisFullError" class="meta">{{ tApp('redis.inspector.failedLoadFull', { message: redisFullError }) }}</div>

                <template v-if="activeView">
                  <div v-if="activeView.isEmpty" class="redis-empty-state">
                    <span class="meta">{{ tApp(emptyCopyKey) }}</span>
                  </div>
                  <div v-else-if="previewKind === 'string'" class="redis-string-preview">
                    <div class="redis-string-chips">
                      <span
                        v-if="stringIsBinary"
                        class="redis-string-chip redis-string-chip--binary"
                        :title="tApp('redis.inspector.chipBinaryHint')"
                      >{{ tApp('redis.inspector.chipBinary') }}</span>
                      <span
                        v-if="isStringJson && effectiveStringViewMode === 'text'"
                        class="redis-string-chip redis-string-chip--json"
                        :class="previewAccent.pill"
                      >{{ tApp('redis.inspector.chipJson') }}</span>
                      <button
                        v-if="isStringJson && effectiveStringViewMode === 'text'"
                        type="button"
                        class="btn ghost mini"
                        @click="showPrettyJson = !showPrettyJson"
                      >
                        {{ showPrettyJson ? tApp('redis.inspector.jsonToggleRaw') : tApp('redis.inspector.jsonTogglePretty') }}
                      </button>
                      <div class="redis-string-view-toggle" role="group" :aria-label="tApp('redis.inspector.viewModeLabel')">
                        <button
                          type="button"
                          class="btn ghost mini"
                          :class="{ 'is-active': stringViewMode === 'auto' }"
                          @click="stringViewMode = 'auto'"
                        >{{ tApp('redis.inspector.viewModeAuto') }}</button>
                        <button
                          type="button"
                          class="btn ghost mini"
                          :class="{ 'is-active': stringViewMode === 'text' }"
                          @click="stringViewMode = 'text'"
                        >{{ tApp('redis.inspector.viewModeText') }}</button>
                        <button
                          type="button"
                          class="btn ghost mini"
                          :class="{ 'is-active': stringViewMode === 'hex' }"
                          @click="stringViewMode = 'hex'"
                        >{{ tApp('redis.inspector.viewModeHex') }}</button>
                      </div>
                    </div>
                    <template v-if="effectiveStringViewMode === 'hex'">
                      <pre
                        class="redis-value redis-value--hex"
                        data-testid="redis-string-hex"
                      >{{ stringHexDump || '-' }}</pre>
                      <p
                        v-if="stringPreviewSource?.valueB64Truncated"
                        class="meta redis-string-hex-note"
                      >{{ tApp('redis.inspector.hexTruncated') }}</p>
                    </template>
                    <pre
                      v-else
                      class="redis-value"
                    >{{ (isStringJson && showPrettyJson && parsedJsonPretty) ? parsedJsonPretty : (activeView.rows?.[0]?.[0] ?? '-') }}</pre>
                  </div>
                  <table v-else class="redis-preview-table">
                    <thead>
                      <tr>
                        <th
                          v-for="head in activeView.headers"
                          :key="head"
                          :class="[previewAccent.headerBg, previewAccent.headerText]"
                        >{{ head }}</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="(row, idx) in activeView.rows" :key="idx">
                        <td v-for="(cell, cidx) in row" :key="cidx" :class="cidx === 0 && activeView.headers.length > 1 ? 'redis-cell-key' : 'redis-cell-value'">{{ cell }}</td>
                      </tr>
                    </tbody>
                  </table>
                </template>
                <div v-else class="meta">{{ tApp('redis.inspector.noPreviewItems') }}</div>
              </template>
            </div>
          </div>
          <div v-else class="redis-preview">
            <div class="redis-preview-head">
              <span>{{ tApp('redis.inspector.previewTab') }}</span>
              <div class="redis-preview-actions">
                <span class="meta" v-if="!store.selectedEntity">{{ tApp('redis.inspector.selectKeyToPreview') }}</span>
                <span class="meta" v-else>{{ tApp('redis.inspector.noPreviewAvailable') }}</span>
              </div>
            </div>
            <div class="redis-preview-body">
              <div class="meta" v-if="!store.selectedEntity">{{ tApp('redis.inspector.meta.selectKeyToInspect') }}</div>
              <div class="meta" v-else>{{ tApp('redis.inspector.noPreviewItems') }}</div>
            </div>
          </div>
        </div>

        <div
          v-else
          class="redis-inspector-pane redis-command-output"
          data-testid="redis-inspector-output"
        >
          <div v-if="hasMultiResults" class="result-tabs" role="tablist" :aria-label="tApp('redis.inspector.resultTabs')">
            <button
              v-for="tab in multiResults"
              :key="tab.id"
              class="result-tab"
              type="button"
              role="tab"
              :aria-selected="tab.id === activeMultiResultId"
              :class="{ active: tab.id === activeMultiResultId }"
              @click="selectMultiResult(tab.id)"
            >
              <span class="result-tab-dot" :class="tab.statusType"></span>
              <span class="result-tab-label">{{ tab.label }}</span>
            </button>
            <button class="btn ghost mini result-tabs-clear" type="button" @click="clearMultiResults">{{ tApp('redis.inspector.clearTabs') }}</button>
          </div>
          <div class="redis-preview">
            <div class="redis-preview-head">
              <span>{{ tApp('redis.inspector.commandOutput') }}</span>
              <div class="redis-preview-actions">
                <span
                  v-if="showStatusChip"
                  class="statement-status"
                  :class="statusType"
                  :title="statusMessage"
                >
                  {{ statusMessage }}
                </span>
                <span v-if="resultMeta" class="meta">{{ resultMeta }}</span>
                <button
                  v-if="resultRows.length"
                  class="btn ghost mini"
                  type="button"
                  data-testid="redis-result-copy"
                  @click="copyRedisResults"
                >
                  Copy
                </button>
              </div>
            </div>
            <div class="redis-preview-body">
              <pre
                v-if="resultRows.length"
                class="redis-result-output"
                data-testid="redis-command-output"
              >{{ redisResultText }}</pre>
              <div v-else class="meta">{{ tApp('redis.inspector.noOutput') }}</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
