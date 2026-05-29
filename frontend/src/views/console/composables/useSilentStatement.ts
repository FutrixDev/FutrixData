import { ref, type Ref } from 'vue'

export function useSilentStatement(statement: Ref<string>) {
  const ignoreStatementChange = ref(false)

  const setStatementSilently = (value: string) => {
    ignoreStatementChange.value = true
    statement.value = value
    Promise.resolve().then(() => {
      ignoreStatementChange.value = false
    })
  }

  return { ignoreStatementChange, setStatementSilently }
}
