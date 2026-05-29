import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'

import EmbeddingConfigForm from '@/components/EmbeddingConfigForm.vue'
import { api } from '@/services/api'
import { resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

describe('EmbeddingConfigForm', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    resetAppI18nForTest()
    setAppLocale('en')
    vi.spyOn(api, 'listEmbeddingConfigs').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows a translated validation message when model is missing', async () => {
    vi.spyOn(api, 'listEmbeddingProviders').mockResolvedValue({
      openai: {
        name: 'OpenAI',
        baseUrl: 'https://api.openai.com/v1',
        defaultModel: '',
        models: [],
      },
    } as any)
    const createSpy = vi.spyOn(api, 'createEmbeddingConfig').mockResolvedValue({ id: 'emb_1' } as any)

    const wrapper = mount(EmbeddingConfigForm, {
      props: { mode: 'create' },
    })
    await flushPromises()

    await wrapper.find('#emb-name').setValue('Embedding config')
    await wrapper.findAll('button').find((btn) => btn.text() === tApp('common.save'))!.trigger('click')
    await flushPromises()

    expect(createSpy).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain(tApp('validation.modelRequired'))
    expect(wrapper.text()).not.toContain('validation.modelRequired')
  })

  it('disables autocorrect helpers on user-editable embedding inputs', async () => {
    vi.spyOn(api, 'listEmbeddingProviders').mockResolvedValue({
      openai: {
        name: 'OpenAI',
        baseUrl: 'https://api.openai.com/v1',
        defaultModel: 'text-embedding-3-small',
        models: ['text-embedding-3-small'],
      },
      custom: {
        name: 'Custom',
        baseUrl: '',
        defaultModel: '',
        models: [],
      },
    } as any)

    const wrapper = mount(EmbeddingConfigForm, {
      props: { mode: 'create' },
    })
    await flushPromises()

    for (const selector of ['#emb-name', '#emb-apikey']) {
      const input = wrapper.get(selector)
      expect(input.attributes('autocapitalize')).toBe('off')
      expect(input.attributes('autocomplete')).toBe('off')
      expect(input.attributes('autocorrect')).toBe('off')
      expect(input.attributes('spellcheck')).toBe('false')
    }

    await wrapper.get('#emb-provider').setValue('custom')
    await flushPromises()

    const baseUrlInput = wrapper.get('#emb-baseurl')
    expect(baseUrlInput.attributes('autocapitalize')).toBe('off')
    expect(baseUrlInput.attributes('autocomplete')).toBe('off')
    expect(baseUrlInput.attributes('autocorrect')).toBe('off')
    expect(baseUrlInput.attributes('spellcheck')).toBe('false')
  })
})
