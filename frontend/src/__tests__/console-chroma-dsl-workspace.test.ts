import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import ConsoleChromaDslWorkspace from '@/views/console/components/chroma-dsl/ConsoleChromaDslWorkspace.vue'

describe('ConsoleChromaDslWorkspace', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
  })

  it('preserves valid raw request edits when running a search via live DSL editor', async () => {
    const wrapper = mount(ConsoleChromaDslWorkspace, {
      global: { plugins: [createPinia()] },
      props: {
        datasourceId: 'ds_chroma',
        statement: 'POST /collections/docs/query\n{"n_results":50,"query_texts":["alpha"],"include":["documents","metadatas","distances"]}',
        selectedTargetPath: 'docs',
        collectionDimension: 0,
        canExecute: true,
      },
    })

    // Open the live DSL editor
    await wrapper.get('#chroma-live-dsl-toggle').setValue(true)

    // Edit the DSL body directly in the textarea
    const dslEditor = wrapper.get('.chroma-dsl-editor')
    await dslEditor.setValue('{\n  "n_results": 5,\n  "query_texts": ["alpha"],\n  "include": ["documents", "distances"],\n  "custom_flag": true\n}')

    await wrapper.get('[data-testid="chroma-dsl-run-search"]').trigger('click')

    const executePayload = String(wrapper.emitted('execute')?.[0]?.[0] || '')

    expect(executePayload).toContain('"custom_flag": true')
    expect(executePayload).toContain('"query_texts": [')
  })

  it('preserves query_texts when rebuilding a structured request', async () => {
    const wrapper = mount(ConsoleChromaDslWorkspace, {
      global: { plugins: [createPinia()] },
      props: {
        datasourceId: 'ds_chroma',
        statement: 'POST /collections/docs/query\n{"n_results":50,"query_texts":["alpha"],"include":["documents","metadatas","distances"]}',
        selectedTargetPath: 'docs',
        collectionDimension: 0,
        canExecute: true,
      },
    })

    await wrapper.get('[data-testid="chroma-dsl-chip-metadatas"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="chroma-dsl-run-search"]').trigger('click')

    const executePayload = String(wrapper.emitted('execute')?.[0]?.[0] || '')

    expect(executePayload).toContain('"query_texts": [')
    expect(executePayload).toContain('"alpha"')
    expect(executePayload).not.toContain('"query_embeddings"')
  })

  it('runs the live DSL request directly in text mode instead of recomputing embeddings', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useAppStore()
    store.embeddingConfigs = [
      {
        id: 'emb-openai',
        name: 'OpenAI embeddings',
        provider: 'openai',
        model: 'text-embedding-3-small',
        purpose: 'embedding',
      } as any,
    ]

    const computeSpy = vi.spyOn(api, 'computeEmbeddingForSearch').mockResolvedValue([0.1, 0.2, 0.3])

    const wrapper = mount(ConsoleChromaDslWorkspace, {
      global: { plugins: [pinia] },
      props: {
        datasourceId: 'ds_chroma',
        statement: 'POST /collections/docs/query\n{"n_results":3,"query_texts":["alpha"],"include":["documents","metadatas","distances"]}',
        selectedTargetPath: 'docs',
        collectionDimension: 1536,
        canExecute: true,
      },
    })

    await wrapper.get('[data-testid="chroma-dsl-mode-query"]').trigger('click')
    const searchModeButtons = wrapper.findAll('.chroma-dsl-search-mode-chip')
    await searchModeButtons[1]!.trigger('click')
    await wrapper.get('#chroma-live-dsl-toggle').setValue(true)
    const dslEditor = wrapper.get('.chroma-dsl-editor')
    await dslEditor.setValue('{\n  "n_results": 3,\n  "query_texts": ["manual body"],\n  "include": ["documents"]\n}')
    await flushPromises()

    const runButton = wrapper.get('[data-testid="chroma-dsl-run-search"]')
    expect((runButton.element as HTMLButtonElement).disabled).toBe(false)

    await runButton.trigger('click')

    expect(computeSpy).not.toHaveBeenCalled()
    expect(String(wrapper.emitted('execute')?.[0]?.[0] || '')).toContain('"manual body"')
  })

  it('keeps max_distance = 0 in the generated request body', async () => {
    const wrapper = mount(ConsoleChromaDslWorkspace, {
      global: { plugins: [createPinia()] },
      props: {
        datasourceId: 'ds_chroma',
        statement: '',
        selectedTargetPath: 'docs',
        collectionDimension: 0,
        canExecute: true,
      },
    })

    await wrapper.get('[data-testid="chroma-dsl-mode-query"]').trigger('click')
    await wrapper.get('[data-testid="chroma-dsl-query-embeddings"]').setValue('[0.1, 0.2, 0.3]')
    await wrapper.get('[data-testid="chroma-dsl-max-distance"]').setValue('0')
    await flushPromises()
    await wrapper.get('[data-testid="chroma-dsl-run-search"]').trigger('click')

    const executePayload = String(wrapper.emitted('execute')?.[0]?.[0] || '')

    expect(executePayload).toContain('"max_distance": 0')
  })

  it('does not throw on malformed encoded collection ids', () => {
    const mountWorkspace = () => mount(ConsoleChromaDslWorkspace, {
      global: { plugins: [createPinia()] },
      props: {
        datasourceId: 'ds_chroma',
        statement: 'POST /collections/%E0%A4%A/query\n{"n_results":50,"query_texts":["alpha"],"include":["documents"]}',
        selectedTargetPath: 'docs',
        collectionDimension: 0,
        canExecute: true,
      },
    })

    expect(mountWorkspace).not.toThrow()
    const wrapper = mountWorkspace()
    expect(wrapper.find('[data-testid="chroma-dsl-workspace"]').exists()).toBe(true)
  })

  it('parses API-prefixed Chroma request lines without losing target or mode', async () => {
    const wrapper = mount(ConsoleChromaDslWorkspace, {
      global: { plugins: [createPinia()] },
      props: {
        datasourceId: 'ds_chroma',
        statement: 'POST /api/v2/tenants/default_tenant/databases/default_database/collections/docs/query\n{"n_results":2,"query_texts":["alpha"],"include":["documents","distances"]}',
        selectedTargetPath: '',
        collectionDimension: 0,
        canExecute: true,
      },
    })

    expect(wrapper.find('[data-testid="chroma-dsl-mode-query"]').classes()).toContain('active')

    await wrapper.get('[data-testid="chroma-dsl-chip-metadatas"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="chroma-dsl-run-search"]').trigger('click')

    const executePayload = String(wrapper.emitted('execute')?.[0]?.[0] || '')

    expect(executePayload).toContain('POST /collections/docs/query')
    expect(executePayload).toContain('"query_texts": [')
    expect(executePayload).toContain('"alpha"')
  })
})
