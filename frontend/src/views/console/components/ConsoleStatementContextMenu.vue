<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  ChevronRight,
  Play,
  Copy,
  History,
  Lightbulb,
  Keyboard,
} from 'lucide-vue-next'
import { tApp } from '@/modules/i18n/appI18n'

const props = withDefaults(defineProps<{
  visible: boolean
  x: number
  y: number
  hasSelection: boolean
  hasContent: boolean
  canExecute: boolean
  aiShortcutPreset?: 'default' | 'redis-help-only'
  showHistory?: boolean
  executeLabel?: string
  copyLabel?: string
  historyLabel?: string
  testIdPrefix?: string
  askAiShortcutLabel?: string
  askAiShortcutDescription?: string
  askAiLabel?: string
  assistantLabel?: string
  aiSuggestionsLabel?: string
  customPromptPlaceholder?: string
  enterToSendLabel?: string
  sendMessageLabel?: string
}>(), {
  aiShortcutPreset: 'default',
  showHistory: true,
  testIdPrefix: 'statement-context',
})

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'execute'): void
  (e: 'copy'): void
  (e: 'history'): void
  (e: 'ask-ai', swift?: string): void
}>()

const menuRef = ref<HTMLElement | null>(null)
const tooltipRef = ref<HTMLElement | null>(null)
const aiTooltipVisible = ref(false)
const customPrompt = ref('')
const customInputRef = ref<HTMLTextAreaElement | null>(null)
const isComposing = ref(false)
const tooltipSide = ref<'left' | 'right'>('right')
const tooltipTopOffset = ref(0)
const tooltipMaxHeight = ref(320)
let aiTooltipTimeout: ReturnType<typeof setTimeout> | null = null
const TOOLTIP_MARGIN = 8
const TOOLTIP_GAP = 16
const DEFAULT_TOOLTIP_WIDTH = 288
const DEFAULT_TOOLTIP_HEIGHT = 320
const MIN_TOOLTIP_HEIGHT = 160

const clampAxis = (value: number, min: number, max: number) => {
  return Math.max(min, Math.min(max, value))
}

const syncTooltipPlacement = () => {
  if (!aiTooltipVisible.value) return
  const menuEl = menuRef.value
  if (!menuEl) return

  const viewportWidth = Math.max(0, Number(window.innerWidth || 0))
  const viewportHeight = Math.max(0, Number(window.innerHeight || 0))
  const menuRect = menuEl.getBoundingClientRect()
  const menuWidth = Math.max(160, Number(menuEl.offsetWidth || menuRect.width || 0))
  const tooltipWidth = Math.max(
    MIN_TOOLTIP_HEIGHT,
    Number(tooltipRef.value?.offsetWidth || DEFAULT_TOOLTIP_WIDTH),
  )
  const tooltipHeight = Math.max(
    MIN_TOOLTIP_HEIGHT,
    Number(tooltipRef.value?.offsetHeight || DEFAULT_TOOLTIP_HEIGHT),
  )

  const spaceRight = viewportWidth - (menuRect.left + menuWidth + TOOLTIP_GAP) - TOOLTIP_MARGIN
  const spaceLeft = menuRect.left - TOOLTIP_GAP - TOOLTIP_MARGIN
  if (spaceRight >= tooltipWidth) {
    tooltipSide.value = 'right'
  } else if (spaceLeft >= tooltipWidth) {
    tooltipSide.value = 'left'
  } else {
    tooltipSide.value = spaceRight >= spaceLeft ? 'right' : 'left'
  }

  const minTop = TOOLTIP_MARGIN
  const maxTop = viewportHeight - TOOLTIP_MARGIN - tooltipHeight
  const resolvedTop = maxTop < minTop
    ? minTop
    : clampAxis(menuRect.top, minTop, maxTop)

  tooltipTopOffset.value = Math.round(resolvedTop - menuRect.top)
  const availableHeight = maxTop < minTop
    ? viewportHeight - TOOLTIP_MARGIN * 2
    : viewportHeight - TOOLTIP_MARGIN - resolvedTop
  tooltipMaxHeight.value = Math.max(MIN_TOOLTIP_HEIGHT, Math.round(availableHeight))
}

const scheduleTooltipPlacement = () => {
  if (!aiTooltipVisible.value) return
  void nextTick(() => {
    syncTooltipPlacement()
  })
}

const handleMouseEnterAi = () => {
  if (aiTooltipTimeout) {
    clearTimeout(aiTooltipTimeout)
    aiTooltipTimeout = null
  }
  aiTooltipVisible.value = true
  scheduleTooltipPlacement()
  // Focus logic handled in template or separate focus call
  nextTick(() => {
    customInputRef.value?.focus()
  })
}

const handleMouseLeaveAi = () => {
  aiTooltipTimeout = setTimeout(() => {
    aiTooltipVisible.value = false
  }, 300)
}

const handleCompositionStart = () => {
  isComposing.value = true
}

const handleCompositionEnd = () => {
  isComposing.value = false
}

const handleInput = () => {
  const el = customInputRef.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${el.scrollHeight}px`
}

const handleEnter = (e: KeyboardEvent) => {
  if (isComposing.value || e.isComposing) return
  if (e.shiftKey) return
  e.preventDefault()
  submitCustomPrompt()
}

const submitCustomPrompt = () => {
  if (!customPrompt.value.trim()) return
  emit('ask-ai', customPrompt.value)
  emit('close')
  customPrompt.value = ''
  // Reset height
  nextTick(() => {
    if (customInputRef.value) {
      customInputRef.value.style.height = 'auto'
    }
  })
}

const askAi = () => {
  emit('ask-ai')
  emit('close')
}

const askAiSwift = (swift: string) => {
  emit('ask-ai', swift)
  emit('close')
}

const showTooltipLeft = computed(() => {
  return tooltipSide.value === 'left'
})

const tooltipPositionClass = computed(() =>
  showTooltipLeft.value ? 'right-full mr-4 origin-right' : 'left-full ml-4 origin-left',
)
const tooltipStyle = computed(() => ({
  top: `${tooltipTopOffset.value}px`,
  maxHeight: `${tooltipMaxHeight.value}px`,
}))

const askAiLabelText = computed(() => props.askAiLabel || tApp('context.askAi'))
const assistantLabelText = computed(() => props.assistantLabel || tApp('context.smartAssistant'))
const aiSuggestionsLabelText = computed(() => props.aiSuggestionsLabel || tApp('context.aiSuggestions'))
const executeLabelText = computed(() => props.executeLabel || tApp('context.executeSelection'))
const copyLabelText = computed(() => props.copyLabel || tApp('context.copySnippet'))
const historyLabelText = computed(() => props.historyLabel || tApp('context.viewHistory'))
const askAiShortcutLabelText = computed(() => props.askAiShortcutLabel || tApp('context.redisCommandHelp'))
const askAiShortcutDescriptionText = computed(
  () => props.askAiShortcutDescription || tApp('context.redisCommandHelpDesc'),
)
const customPromptPlaceholderText = computed(
  () => props.customPromptPlaceholder || tApp('context.customPlaceholder'),
)
const enterToSendLabelText = computed(() => props.enterToSendLabel || tApp('context.enterToSend'))
const sendMessageLabelText = computed(() => props.sendMessageLabel || tApp('context.sendMessage'))

const resolveTestId = (suffix: string) => `${props.testIdPrefix}-${suffix}`

type AiShortcutItem = {
  id: string
  testId: string
  prompt: string
  label: string
  description: string
  dotClass: string
}

const defaultAiShortcutItems = computed<AiShortcutItem[]>(() => [
  {
    id: 'explain-logic',
    testId: resolveTestId('ask-ai-explain-logic'),
    prompt: tApp('context.explainLogic'),
    label: tApp('context.explainLogic'),
    description: tApp('context.explainLogicDesc'),
    dotClass: 'bg-primary shadow shadow-primary',
  },
  {
    id: 'optimize-performance',
    testId: resolveTestId('ask-ai-optimize-performance'),
    prompt: tApp('context.optimizePerformance'),
    label: tApp('context.optimizePerformance'),
    description: tApp('context.optimizePerformanceDesc'),
    dotClass: 'bg-orange-400',
  },
  {
    id: 'debug-error',
    testId: resolveTestId('ask-ai-debug-error'),
    prompt: tApp('context.debugError'),
    label: tApp('context.debugError'),
    description: tApp('context.debugErrorDesc'),
    dotClass: 'bg-blue-400',
  },
])

const redisHelpAiShortcutItem = computed<AiShortcutItem>(() => ({
  id: 'redis-help',
  testId: resolveTestId('ask-ai-redis-help'),
  prompt: askAiShortcutLabelText.value,
  label: askAiShortcutLabelText.value,
  description: askAiShortcutDescriptionText.value,
  dotClass: 'bg-primary shadow shadow-primary',
}))

const aiShortcutItems = computed<AiShortcutItem[]>(() => {
  if (props.aiShortcutPreset === 'redis-help-only') {
    return [redisHelpAiShortcutItem.value]
  }
  return defaultAiShortcutItems.value
})

watch(
  () => [aiTooltipVisible.value, props.x, props.y],
  ([visible]) => {
    if (!visible) return
    scheduleTooltipPlacement()
  },
)

onMounted(() => {
  window.addEventListener('resize', syncTooltipPlacement)
})

onBeforeUnmount(() => {
  if (aiTooltipTimeout) {
    clearTimeout(aiTooltipTimeout)
    aiTooltipTimeout = null
  }
  window.removeEventListener('resize', syncTooltipPlacement)
})
</script>

<template>
  <div
    v-if="visible"
    ref="menuRef"
    class="fixed z-50 select-none"
    :data-testid="resolveTestId('menu')"
    :style="{ left: `${x}px`, top: `${y}px` }"
    @mousedown.stop
    @click.stop
  >
    <div class="relative">
      <!-- Context Menu -->
      <div
        class="w-64 bg-white dark:bg-[#1a1a1a] border border-gray-200 dark:border-white/10 rounded-xl shadow-menu backdrop-blur-xl flex flex-col p-1.5 animate-in fade-in zoom-in-95 duration-200 origin-top-left"
      >
        <!-- Section 1: AI Actions -->
        <div class="relative group" @mouseenter="handleMouseEnterAi" @mouseleave="handleMouseLeaveAi">
          <button
            :data-testid="resolveTestId('ask-ai')"
            class="w-full text-left px-3 py-2.5 rounded-lg flex items-center justify-between bg-gray-50 dark:bg-white/5 hover:bg-gray-100 dark:hover:bg-white/10 border border-primary/20 hover:border-primary transition-colors duration-200 group-hover:border-primary cursor-pointer"
            @click="askAi"
          >
            <div class="flex items-center gap-3">
              <div
                class="relative flex items-center justify-center w-6 h-6 rounded bg-primary/10 dark:bg-primary/20 text-primary"
              >
                <span class="ai-composer-icon w-4 h-4 flex items-center justify-center" aria-hidden="true">
                  <svg class="ai-composer-glyph w-4 h-4" viewBox="0 0 24 24" aria-hidden="true">
                    <path
                      d="M12 3.5l2.6 5.5 6 .9-4.3 4.2 1 6-5.3-3-5.3 3 1-6L3.4 9.9l6-.9L12 3.5z"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="1.4"
                      stroke-linejoin="round"
                    />
                  </svg>
                </span>
                <div class="absolute inset-0 bg-primary/20 rounded blur animate-pulse" />
              </div>
              <div>
                <span
                  class="block text-sm font-semibold text-gray-900 dark:text-white group-hover:text-primary transition-colors"
                >
                  {{ askAiLabelText }}
                </span>
                <span class="block text-[10px] text-gray-500 dark:text-white/40">{{ assistantLabelText }}</span>
              </div>
            </div>
            <ChevronRight
              class="w-4 h-4 text-gray-400 dark:text-white/30 group-hover:text-primary transition-colors"
            />
          </button>
          <!-- Connection Line (Hidden bridge for hover safety logic, though timeout handles most) -->
          <div
            v-if="aiTooltipVisible"
            class="absolute top-0 bottom-0 w-8 bg-transparent"
            :class="showTooltipLeft ? 'right-full' : 'left-full'"
          />
        </div>

        <div class="h-px bg-gray-200 dark:bg-white/5 my-1.5 mx-2" />

        <!-- Section 2: Core Actions -->
        <button
          :data-testid="resolveTestId('execute')"
          class="w-full text-left px-3 py-2 rounded-lg flex items-center gap-3 text-gray-700 dark:text-white/80 hover:bg-gray-100 dark:hover:bg-white/5 hover:text-gray-900 dark:hover:text-white transition-colors group disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
          :disabled="!canExecute"
          @click="emit('execute')"
        >
          <Play class="w-5 h-5 text-primary" />
          <span class="text-sm font-medium">{{ executeLabelText }}</span>
          <span
            class="ml-auto text-xs text-gray-400 dark:text-white/20 font-mono group-hover:text-gray-500 dark:group-hover:text-white/40"
          >
            Cmd+Enter
          </span>
        </button>

        <button
          :data-testid="resolveTestId('copy')"
          class="w-full text-left px-3 py-2 rounded-lg flex items-center gap-3 text-gray-700 dark:text-white/80 hover:bg-gray-100 dark:hover:bg-white/5 hover:text-gray-900 dark:hover:text-white transition-colors group disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
          :disabled="!hasContent"
          @click="emit('copy')"
        >
          <Copy
            class="w-5 h-5 text-gray-400 dark:text-white/40 group-hover:text-gray-600 dark:group-hover:text-white transition-colors"
          />
          <span class="text-sm font-medium">{{ copyLabelText }}</span>
          <span
            class="ml-auto text-xs text-gray-400 dark:text-white/20 font-mono group-hover:text-gray-500 dark:group-hover:text-white/40"
          >
            Cmd+C
          </span>
        </button>

        <template v-if="showHistory">
          <div class="h-px bg-gray-200 dark:bg-white/5 my-1.5 mx-2" />

          <!-- Section 3: Utility -->
          <button
            :data-testid="resolveTestId('history')"
            class="w-full text-left px-3 py-2 rounded-lg flex items-center gap-3 text-gray-600 dark:text-white/60 hover:bg-gray-100 dark:hover:bg-white/5 hover:text-gray-900 dark:hover:text-white transition-colors cursor-pointer"
            @click="emit('history')"
          >
            <History class="w-5 h-5 text-gray-400 dark:text-white/40" />
            <span class="text-sm font-medium">{{ historyLabelText }}</span>
          </button>
        </template>
      </div>

      <!-- AI Preview Tooltip -->
      <div
        v-if="aiTooltipVisible"
        ref="tooltipRef"
        :data-testid="resolveTestId('ask-ai-tooltip')"
        class="w-72 bg-white/95 dark:bg-[#151515]/90 border border-primary/20 rounded-xl shadow-glow backdrop-blur-2xl p-4 flex flex-col gap-3 animate-in fade-in slide-in-from-left-2 duration-200 absolute overflow-y-auto overflow-x-hidden ask-ai-tooltip-scrollless"
        :class="tooltipPositionClass"
        :style="tooltipStyle"
        @mouseenter="handleMouseEnterAi"
        @mouseleave="handleMouseLeaveAi"
      >
        <!-- Decorative Glow -->
        <div
          class="absolute -inset-0.5 bg-gradient-to-b from-primary/10 to-transparent rounded-xl -z-10 blur-sm"
        />

        <div class="flex items-center gap-2 mb-1">
          <Lightbulb class="w-4 h-4 text-primary" />
          <span class="text-xs font-bold text-primary uppercase tracking-wider">{{ aiSuggestionsLabelText }}</span>
        </div>

        <div class="space-y-2">
          <button
            v-for="shortcut in aiShortcutItems"
            :key="shortcut.id"
            :data-testid="shortcut.testId"
            class="w-full group relative overflow-hidden rounded-lg bg-gray-50 dark:bg-white/5 p-3 text-left transition hover:bg-gray-100 dark:hover:bg-white/10 hover:shadow-lg hover:shadow-primary/5 hover:border-primary/30 border border-transparent cursor-pointer"
            @click="askAiSwift(shortcut.prompt)"
          >
            <div class="relative z-10 flex items-start gap-3">
              <div class="mt-0.5 h-1.5 w-1.5 rounded-full" :class="shortcut.dotClass" />
              <div>
                <p
                  class="text-sm font-medium text-gray-900 dark:text-white group-hover:text-primary transition-colors"
                >
                  {{ shortcut.label }}
                </p>
                <p class="text-xs text-gray-500 dark:text-white/40 mt-0.5">
                  {{ shortcut.description }}
                </p>
              </div>
            </div>
          </button>
        </div>

        <!-- Bottom hint / Custom Input -->
        <div
          class="mt-1 pt-3 border-t border-gray-200 dark:border-white/5"
          @click.stop
          @mousedown.stop
        >
          <div class="statement-context-mini-composer ai-composer-box">
            <textarea
              v-model="customPrompt"
              ref="customInputRef"
              rows="1"
              :data-testid="resolveTestId('ask-ai-custom')"
              class="statement-context-mini-input ai-composer-input ai-composer-input-area"
              :placeholder="customPromptPlaceholderText"
              @compositionstart="handleCompositionStart"
              @compositionend="handleCompositionEnd"
              @input="handleInput"
              @keydown.enter="handleEnter"
              @keydown.stop
            ></textarea>

            <div class="statement-context-mini-toolbar ai-composer-toolbar">
              <div class="statement-context-mini-hint">
                <Keyboard class="w-3.5 h-3.5 shrink-0" />
                <span>{{ enterToSendLabelText }}</span>
              </div>
              <button
                class="statement-context-mini-send ai-send-circle-btn ai-send-icon"
                type="button"
                :disabled="!customPrompt.trim()"
                :aria-label="sendMessageLabelText"
                @click="submitCustomPrompt"
              >
                <svg
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  aria-hidden="true"
                >
                  <line x1="12" y1="19" x2="12" y2="5"></line>
                  <polyline points="5 12 12 5 19 12"></polyline>
                </svg>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<!-- scoped style needs to be included as well -->
<style scoped>
.shadow-glow {
  box-shadow: 0 0 20px -5px rgba(12, 223, 15, 0.3);
}
.shadow-menu {
  box-shadow: 0 10px 40px -10px rgba(0, 0, 0, 0.8);
}

.ask-ai-tooltip-scrollless {
  scrollbar-width: none;
  -ms-overflow-style: none;
}

.ask-ai-tooltip-scrollless::-webkit-scrollbar {
  width: 0;
  height: 0;
  display: none;
}

.statement-context-mini-composer {
  gap: 6px;
  padding: 8px 10px;
  border-radius: 12px;
  border: 1px solid color-mix(in oklab, #b9c3ad 52%, transparent);
  background: color-mix(in oklab, #ffffff 93%, #f2f6ec);
  box-shadow: 0 6px 14px rgba(40, 52, 24, 0.1);
  transition: border-color 0.18s ease, box-shadow 0.18s ease;
}

.statement-context-mini-composer:focus-within {
  border-color: color-mix(in oklab, #7f9960 44%, #c3ccb6);
  box-shadow:
    0 6px 14px rgba(40, 52, 24, 0.12),
    0 0 0 2px color-mix(in oklab, #8ea772 20%, transparent);
}

.statement-context-mini-input {
  min-height: 24px;
  max-height: 96px;
  font-size: 12px;
  line-height: 1.4;
  padding: 0;
  color: #1f2b18;
}

.statement-context-mini-input::placeholder {
  color: rgba(79, 92, 71, 0.72);
}

.statement-context-mini-toolbar {
  gap: 8px;
}

.statement-context-mini-hint {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: rgba(79, 92, 71, 0.78);
}

.statement-context-mini-send.ai-send-circle-btn,
.statement-context-mini-send.ai-send-icon {
  width: 24px;
  height: 24px;
  border: none;
  background: color-mix(in oklab, #e7d089 65%, #ffffff);
  color: color-mix(in oklab, #476533 72%, #26351c);
  cursor: pointer;
  transition: transform 0.18s ease, box-shadow 0.18s ease, background-color 0.18s ease;
  box-shadow: 0 3px 8px rgba(57, 77, 32, 0.16);
}

.statement-context-mini-send.ai-send-circle-btn:hover:not(:disabled),
.statement-context-mini-send.ai-send-icon:hover:not(:disabled) {
  transform: translateY(-1px) scale(1.02);
  background: color-mix(in oklab, #edd894 80%, #ffffff);
  box-shadow: 0 6px 12px rgba(57, 77, 32, 0.18);
}

.statement-context-mini-send.ai-send-circle-btn:disabled,
.statement-context-mini-send.ai-send-icon:disabled {
  background: #d9ddd3;
  color: rgba(79, 92, 71, 0.55);
  cursor: not-allowed;
  box-shadow: none;
  transform: none;
}

.statement-context-mini-send svg {
  width: 12px;
  height: 12px;
}
</style>
