import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AIConfigPanel from '@/components/AIConfigPanel.vue'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'

describe('AIConfigPanel', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders split columns and toggles long status detail', async () => {
    const store = useAppStore()
    store.aiConfigs = [
      {
        id: 'ai_ok',
        name: 'OpenAI',
        provider: 'openai',
        model: 'gpt-4.1-mini',
        status: 'connected',
        lastLatencyMs: 110,
        lastModelInfo: 'gpt-4.1-mini',
      } as any,
      {
        id: 'ai_bad',
        name: 'Broken',
        provider: 'custom',
        model: '',
        status: 'failed',
        statusDetail: 'x'.repeat(180),
      } as any,
    ]

    const wrapper = mount(AIConfigPanel, {
      props: { visible: true, inline: true, split: true },
      global: { plugins: [pinia] },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Connected')
    expect(wrapper.text()).toContain('Needs attention')

    const badCard = wrapper.findAll('.ai-card').find((card) => card.text().includes('Broken'))
    expect(badCard).toBeTruthy()

    const toggle = badCard!.find('.ai-detail-toggle')
    expect(toggle.exists()).toBe(true)

    const detail = badCard!.find('.status-detail')
    expect(detail.classes()).not.toContain('expanded')

    await toggle.trigger('click')
    await flushPromises()

    expect(badCard!.find('.status-detail').classes()).toContain('expanded')

    wrapper.unmount()
  })

  it('opens action menu, emits edit, and closes on outside click', async () => {
    const store = useAppStore()
    store.aiConfigs = [
      { id: 'ai_ok', name: 'OpenAI', provider: 'openai', model: 'gpt-4.1-mini', status: 'connected' } as any,
    ]

    const wrapper = mount(AIConfigPanel, {
      props: { visible: true, inline: true },
      global: { plugins: [pinia] },
    })
    await flushPromises()

    await wrapper.find('.ai-action-toggle').trigger('click')
    expect(wrapper.find('.ai-action-dropdown').exists()).toBe(true)

    await wrapper.findAll('.ai-action-item')[0]!.trigger('click')
    expect(wrapper.emitted('edit')?.[0]).toEqual(['ai_ok'])
    expect(wrapper.find('.ai-action-dropdown').exists()).toBe(false)

    await wrapper.find('.ai-action-toggle').trigger('click')
    expect(wrapper.find('.ai-action-dropdown').exists()).toBe(true)

    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await flushPromises()

    expect(wrapper.find('.ai-action-dropdown').exists()).toBe(false)

    wrapper.unmount()
  })

  it('deletes config after confirmation', async () => {
    const store = useAppStore()
    store.aiConfigs = [
      { id: 'ai_ok', name: 'OpenAI', provider: 'openai', model: 'gpt-4.1-mini', status: 'connected' } as any,
    ]

    const deleteSpy = vi.spyOn(api, 'deleteAIConfig').mockResolvedValue(true as any)

    const wrapper = mount(AIConfigPanel, {
      props: { visible: true, inline: true },
      global: { plugins: [pinia] },
    })
    await flushPromises()

    await wrapper.find('.ai-action-toggle').trigger('click')
    await wrapper.findAll('.ai-action-item').find((btn) => btn.text() === 'Delete')!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="aiconfig-delete-confirm-dialog"]').exists()).toBe(true)

    await wrapper.find('[data-testid="aiconfig-delete-confirm"]').trigger('click')
    await flushPromises()

    expect(deleteSpy).toHaveBeenCalledWith('ai_ok')
    expect(api.listAIConfigs).toHaveBeenCalled()

    wrapper.unmount()
  })
})
