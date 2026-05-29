import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AIConfigForm from '@/components/AIConfigForm.vue'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'

const providersFixture = {
  openai: {
    name: 'OpenAI',
    baseUrl: 'https://api.openai.com/v1',
    defaultModel: 'gpt-4.1-mini',
    models: ['gpt-4.1-mini', 'gpt-4o-mini'],
  },
  custom: {
    name: 'Custom',
    baseUrl: '',
    defaultModel: '',
    models: [],
  },
}

describe('AIConfigForm', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listAIProviders').mockResolvedValue(providersFixture as any)
    vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('validates required fields on save', async () => {
    const createSpy = vi.spyOn(api, 'createAIConfig').mockResolvedValue({ id: 'ai_1' } as any)

    const wrapper = mount(AIConfigForm, {
      props: { visible: true, mode: 'create', inline: true },
      global: { plugins: [pinia] },
    })
    await flushPromises()

    await wrapper.find('#aiconfig-form-save').trigger('click')
    await flushPromises()

    expect(createSpy).not.toHaveBeenCalled()
    expect(wrapper.find('#aiconfig-form-errors').text()).toContain('Name is required.')

    await wrapper.find('#ai-name').setValue('Production OpenAI')
    await wrapper.find('#aiconfig-form-save').trigger('click')
    await flushPromises()

    expect(createSpy).not.toHaveBeenCalled()
    expect(wrapper.find('#aiconfig-form-errors').text()).toContain('API key is required.')
  })

  it('requires base url for custom provider', async () => {
    vi.spyOn(api, 'createAIConfig').mockResolvedValue({ id: 'ai_1' } as any)

    const wrapper = mount(AIConfigForm, {
      props: { visible: true, mode: 'create', inline: true },
      global: { plugins: [pinia] },
    })
    await flushPromises()

    await wrapper.find('#ai-name').setValue('Custom Provider')
    await wrapper.find('#ai-provider').setValue('custom')
    await flushPromises()

    expect(wrapper.find('#ai-baseurl').exists()).toBe(true)

    await wrapper.find('#ai-apikey').setValue('sk-test')
    await wrapper.find('#ai-baseurl').setValue('')

    await wrapper.find('#aiconfig-form-save').trigger('click')
    await flushPromises()

    expect(wrapper.find('#aiconfig-form-errors').text()).toContain('Base URL is required for custom provider.')
  })

  it('tests connection and renders status detail', async () => {
    const testSpy = vi.spyOn(api, 'testAIConfigPayload').mockResolvedValue({
      connected: true,
      latencyMs: 123,
      modelInfo: 'gpt-4.1-mini',
    } as any)

    const wrapper = mount(AIConfigForm, {
      props: { visible: true, mode: 'create', inline: true },
      global: { plugins: [pinia] },
    })
    await flushPromises()

    await wrapper.find('#ai-name').setValue('Production OpenAI')
    await wrapper.find('#ai-apikey').setValue('sk-test')

    await wrapper.find('#aiconfig-form-test').trigger('click')
    await flushPromises()

    expect(testSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Production OpenAI',
        provider: 'openai',
      }),
    )

    const status = wrapper.find('#aiconfig-form-status')
    expect(status.text()).toContain('Connected')
    expect(status.text()).toContain('gpt-4.1-mini')
    expect(status.text()).toContain('123ms')
    expect(status.find('.status').classes()).toContain('connected')
  })

  it('shows masked api key preview and fetches full key on show', async () => {
    const keySpy = vi.spyOn(api, 'getAIConfigAPIKey').mockResolvedValue('sk-real')

    const store = useAppStore()
    store.aiConfigs = [
      {
        id: 'ai_1',
        name: 'Production OpenAI',
        provider: 'openai',
        baseUrl: providersFixture.openai.baseUrl,
        apiKey: 'sk-o***90e6',
        model: 'gpt-4.1-mini',
        status: 'connected',
      } as any,
    ]

    const wrapper = mount(AIConfigForm, {
      props: { visible: true, mode: 'edit', configId: 'ai_1', inline: true },
      global: { plugins: [pinia] },
    })
    await flushPromises()

    const input = wrapper.find('#ai-apikey')
    expect(input.attributes('type')).toBe('text')
    expect((input.element as HTMLInputElement).value).toBe('sk-o***90e6')

    await wrapper.find('.ai-visibility-toggle').trigger('click')
    await flushPromises()

    expect(keySpy).toHaveBeenCalledWith('ai_1')
    expect((wrapper.find('#ai-apikey').element as HTMLInputElement).value).toBe('sk-real')
  })

  it('uses stored api key for preview test when api key unchanged', async () => {
    const previewSpy = vi.spyOn(api, 'testAIConfigPreview').mockResolvedValue({
      connected: true,
      latencyMs: 123,
      modelInfo: 'gpt-4.1-mini',
    } as any)

    const store = useAppStore()
    store.aiConfigs = [
      {
        id: 'ai_1',
        name: 'Production OpenAI',
        provider: 'openai',
        baseUrl: providersFixture.openai.baseUrl,
        apiKey: 'sk-o***90e6',
        model: 'gpt-4.1-mini',
        status: 'connected',
      } as any,
    ]

    const wrapper = mount(AIConfigForm, {
      props: { visible: true, mode: 'edit', configId: 'ai_1', inline: true },
      global: { plugins: [pinia] },
    })
    await flushPromises()

    await wrapper.find('#aiconfig-form-test').trigger('click')
    await flushPromises()

    expect(previewSpy).toHaveBeenCalledWith(
      'ai_1',
      expect.objectContaining({
        apiKey: '',
      }),
    )
  })
})
