import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import RowMutationDialog from '@/views/console/components/RowMutationDialog.vue'
import { setAppLocale } from '@/modules/i18n/appI18n'

const sampleProps = {
  open: true,
  kind: 'delete' as const,
  tableName: 'users',
  pkSummary: 'id = 42',
  statement: "DELETE FROM `users` WHERE `id` = 42;",
}

describe('RowMutationDialog', () => {
  beforeEach(() => {
    setAppLocale('en')
  })

  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('renders delete dialog with statement and primary key summary', async () => {
    const wrapper = mount(RowMutationDialog, {
      attachTo: document.body,
      props: sampleProps,
    })
    await flushPromises()

    const root = wrapper.find('[data-testid="row-mutation-delete-dialog"]')
    expect(root.exists()).toBe(true)

    expect(wrapper.find('[data-testid="row-mutation-table"]').text()).toBe('users')
    expect(wrapper.find('[data-testid="row-mutation-pk"]').text()).toBe('id = 42')
    expect(wrapper.find('[data-testid="row-mutation-statement"]').text()).toContain('DELETE FROM')

    const confirm = wrapper.find('[data-testid="row-mutation-confirm-delete"]')
    expect(confirm.exists()).toBe(true)
    expect(confirm.classes()).toContain('danger')
  })

  it('renders update dialog with a non-danger confirm button', async () => {
    const wrapper = mount(RowMutationDialog, {
      attachTo: document.body,
      props: {
        ...sampleProps,
        kind: 'update' as const,
        statement: "UPDATE `users` SET `name` = 'neo' WHERE `id` = 42;",
      },
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="row-mutation-update-dialog"]').exists()).toBe(true)
    const confirm = wrapper.find('[data-testid="row-mutation-confirm-update"]')
    expect(confirm.exists()).toBe(true)
    expect(confirm.classes()).not.toContain('danger')
    expect(wrapper.find('[data-testid="row-mutation-statement"]').text()).toContain('UPDATE')
  })

  it('emits confirm and cancel from action buttons', async () => {
    const wrapper = mount(RowMutationDialog, {
      attachTo: document.body,
      props: sampleProps,
    })
    await flushPromises()

    await wrapper.find('[data-testid="row-mutation-confirm-delete"]').trigger('click')
    await wrapper.find('[data-testid="row-mutation-cancel"]').trigger('click')

    expect(wrapper.emitted('confirm')?.length).toBe(1)
    expect(wrapper.emitted('cancel')?.length).toBe(1)
  })

  it('disables action buttons while busy is true', async () => {
    const wrapper = mount(RowMutationDialog, {
      attachTo: document.body,
      props: { ...sampleProps, busy: true },
    })
    await flushPromises()

    const confirm = wrapper.find('[data-testid="row-mutation-confirm-delete"]').element as HTMLButtonElement
    const cancel = wrapper.find('[data-testid="row-mutation-cancel"]').element as HTMLButtonElement
    expect(confirm.disabled).toBe(true)
    expect(cancel.disabled).toBe(true)
  })

  it('emits cancel when pressing Escape while open', async () => {
    const wrapper = mount(RowMutationDialog, {
      attachTo: document.body,
      props: sampleProps,
    })
    await flushPromises()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(wrapper.emitted('cancel')?.length).toBe(1)
  })

  it('renders Chinese strings when locale is zh', async () => {
    setAppLocale('zh')
    const wrapper = mount(RowMutationDialog, {
      attachTo: document.body,
      props: sampleProps,
    })
    await flushPromises()

    expect(wrapper.find('h4').text()).toContain('删除')
    const confirm = wrapper.find('[data-testid="row-mutation-confirm-delete"]')
    expect(confirm.text()).toContain('删除')
  })

  it('does not render when open is false', async () => {
    const wrapper = mount(RowMutationDialog, {
      attachTo: document.body,
      props: { ...sampleProps, open: false },
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="row-mutation-delete-dialog"]').exists()).toBe(false)
  })
})
