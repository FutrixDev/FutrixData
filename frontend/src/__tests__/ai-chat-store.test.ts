import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAiChatStore } from '@/stores/ai-chat'

beforeEach(() => {
  const data = new Map<string, string>()
  vi.stubGlobal('localStorage', {
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => { data.set(key, value) },
    removeItem: (key: string) => { data.delete(key) },
    clear: () => { data.clear() },
    key: (index: number) => Array.from(data.keys())[index] ?? null,
    get length() { return data.size },
  } as Storage)
  setActivePinia(createPinia())
  if (typeof localStorage?.clear === 'function') {
    localStorage.clear()
  }
})

describe('ai chat store', () => {
  it('does not expose autoExecuteRiskLevels on prefs after trust-level migration', () => {
    const store = useAiChatStore()
    expect((store.prefs as any).autoExecuteRiskLevels).toBeUndefined()
  })

  it('strips stale autoExecuteRiskLevels entries from legacy localStorage', () => {
    localStorage.setItem('fd_ai_chat_prefs', JSON.stringify({
      defaultOpen: true,
      retention: 12,
      autoExecuteRiskLevels: ['low'],
    }))

    const store = useAiChatStore()

    expect(store.prefs.defaultOpen).toBe(true)
    expect(store.prefs.retention).toBe(12)
    expect((store.prefs as any).autoExecuteRiskLevels).toBeUndefined()
    // Only ["low"] was already the safe default, so no notice is surfaced.
    expect(store.legacyAutoExecuteNotice).toBeNull()
    // And the cleaned-up prefs blob no longer carries the legacy key.
    const persisted = JSON.parse(localStorage.getItem('fd_ai_chat_prefs') || '{}')
    expect(persisted.autoExecuteRiskLevels).toBeUndefined()
  })

  it('surfaces a migration notice when legacy autoExecuteRiskLevels went beyond the default', () => {
    localStorage.setItem('fd_ai_chat_prefs', JSON.stringify({
      defaultOpen: false,
      retention: 50,
      autoExecuteRiskLevels: ['low', 'medium', 'high'],
    }))

    const store = useAiChatStore()

    expect(store.legacyAutoExecuteNotice).not.toBeNull()
    expect(store.legacyAutoExecuteNotice?.levels).toEqual(['low', 'medium', 'high'])
    expect(store.legacyAutoExecuteNotice?.strict).toBe(false)
    // Notice is persisted so it survives reloads until dismissed.
    const stored = JSON.parse(localStorage.getItem('fd_ai_chat_autoexec_migration_v1') || '{}')
    expect(stored.levels).toEqual(['low', 'medium', 'high'])
    // Legacy key is stripped from the prefs blob.
    const persisted = JSON.parse(localStorage.getItem('fd_ai_chat_prefs') || '{}')
    expect(persisted.autoExecuteRiskLevels).toBeUndefined()
  })

  it('flags strict legacy (autoExecuteRiskLevels=[]) so it does not silently downgrade', () => {
    // Empty list meant "auto-run nothing" under the old global pref. The new
    // default trust `cautious` auto-runs low-risk reads, so we must surface
    // a dedicated notice rather than pretending the old setting carried over.
    localStorage.setItem('fd_ai_chat_prefs', JSON.stringify({
      defaultOpen: false,
      retention: 50,
      autoExecuteRiskLevels: [],
    }))

    const store = useAiChatStore()

    expect(store.legacyAutoExecuteNotice).not.toBeNull()
    expect(store.legacyAutoExecuteNotice?.levels).toEqual([])
    expect(store.legacyAutoExecuteNotice?.strict).toBe(true)
    const stored = JSON.parse(localStorage.getItem('fd_ai_chat_autoexec_migration_v1') || '{}')
    expect(stored.strict).toBe(true)
  })

  it('persists the strict flag across reloads', () => {
    localStorage.setItem(
      'fd_ai_chat_autoexec_migration_v1',
      JSON.stringify({ levels: [], strict: true }),
    )

    const store = useAiChatStore()

    expect(store.legacyAutoExecuteNotice?.strict).toBe(true)
    expect(store.legacyAutoExecuteNotice?.levels).toEqual([])
  })

  it('dismissLegacyAutoExecuteNotice clears the notice and its persisted marker', () => {
    localStorage.setItem('fd_ai_chat_prefs', JSON.stringify({
      autoExecuteRiskLevels: ['medium'],
    }))

    const store = useAiChatStore()
    expect(store.legacyAutoExecuteNotice).not.toBeNull()

    store.dismissLegacyAutoExecuteNotice()

    expect(store.legacyAutoExecuteNotice).toBeNull()
    expect(localStorage.getItem('fd_ai_chat_autoexec_migration_v1')).toBeNull()
  })

  it('trims conversations by retention limit', () => {
    const store = useAiChatStore()
    store.setRetentionLimit(2)
    const a = store.createConversation('First')
    const b = store.createConversation('Second')
    const c = store.createConversation('Third')
    expect(store.conversations.length).toBe(2)
    expect(store.conversations.map((c) => c.id)).toEqual([b.id, c.id])
    expect(store.activeId).toBe(c.id)
    expect(store.messagesById[a.id]).toBeUndefined()
  })

  it('deletes active conversation and falls back', () => {
    const store = useAiChatStore()
    const a = store.createConversation('First')
    const b = store.createConversation('Second')
    store.setActive(a.id)
    store.deleteConversation(a.id)
    expect(store.activeId).toBe(b.id)
  })
})
