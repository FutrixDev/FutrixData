import type { VueWrapper } from '@vue/test-utils'
import { flushPromises } from '@vue/test-utils'

export const selectDatasourceType = async (wrapper: VueWrapper, label: string) => {
  await wrapper.find('#ds-type').trigger('click')
  await flushPromises()

  const options = wrapper.findAll('.ds-type-select-option')
  const target = options.find((option) => option.text().includes(label))
  if (!target) {
    throw new Error(`Datasource type option not found: ${label}`)
  }

  await target.trigger('click')
  await flushPromises()
}
