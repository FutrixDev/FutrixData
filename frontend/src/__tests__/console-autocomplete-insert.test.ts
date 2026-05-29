import { computed, defineComponent, ref } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'

import { useAppStore } from '@/stores/app'
import { useAutocomplete } from '@/views/console/composables/useAutocomplete'

describe('console autocomplete insert behavior', () => {
  it('inserts valid bracket-style mongo collection accessor', async () => {
    setActivePinia(createPinia())
    const store = useAppStore()
    store.current = { id: 'ds_mongo', type: 'mongodb' } as any
    store.entities = ['users']

    const statement = ref('db["us')
    const textarea = document.createElement('textarea')
    textarea.value = statement.value
    textarea.selectionStart = statement.value.length
    textarea.selectionEnd = statement.value.length

    let autocompleteApi: ReturnType<typeof useAutocomplete> | null = null

    const Host = defineComponent({
      setup() {
        autocompleteApi = useAutocomplete({
          statement,
          statementInput: ref(textarea),
          statementShell: ref(document.createElement('div')),
          entityDetail: ref(null),
          isMongo: computed(() => true),
          isElastic: computed(() => false),
          isSQL: computed(() => false),
        })
        return {}
      },
      template: '<div />',
    })

    mount(Host)

    const getAutocompleteSuggestions = autocompleteApi!.getAutocompleteSuggestions
    const showAutocomplete = autocompleteApi!.showAutocomplete
    const selectAutocompleteItem = autocompleteApi!.selectAutocompleteItem

    const suggestion = getAutocompleteSuggestions(statement.value, statement.value.length)
    expect(suggestion).not.toBeNull()

    const usersItem = suggestion?.items.find((item) => item.label === 'users')
    expect(usersItem).toBeTruthy()

    showAutocomplete(
      suggestion?.items || [],
      suggestion?.title || '',
      suggestion?.insertStart || 0,
      suggestion?.insertEnd || 0,
      suggestion?.prefix || '',
    )
    selectAutocompleteItem(usersItem!)
    await Promise.resolve()

    expect(statement.value).toBe('db["users"].')
  })
})
