import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it } from 'vitest'
import AiQuickPrompt from '@/components/ai/AiQuickPrompt.vue'
import { useAppStore } from '@/stores/app'

const makeDatasource = (id: string, name: string, type: any) => ({
  id,
  name,
  type,
  host: '',
  port: 0,
})

describe('ai quick prompt', () => {
  it('emits send when submitting', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const wrapper = mount(AiQuickPrompt, {
      props: { open: true, x: 10, y: 10 },
      global: { plugins: [pinia] },
    })
    await wrapper.find('input').setValue('hello')
    await wrapper.find('form').trigger('submit')
    expect(wrapper.emitted('send')?.[0]).toEqual(['hello', []])
  })

  it('supports keyboard navigation in context dropdown', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any

    const wrapper = mount(AiQuickPrompt, {
      props: { open: true, x: 10, y: 10 },
      global: { plugins: [pinia] },
    })
    const input = wrapper.find('input')
    await input.setValue('@')
    await input.trigger('keydown', { key: 'ArrowDown' })
    const items = wrapper.findAll('.ai-context-item')
    expect(items.length).toBeGreaterThan(0)
    expect(items[1].classes()).toContain('active')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.findAll('.ai-context-chip').length).toBe(1)
  })
})
