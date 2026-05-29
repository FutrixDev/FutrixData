import { computed, type ComputedRef, type Ref } from 'vue'

type Template = { label: string; statement: string }

type Params = {
  store: any
  isRedis: ComputedRef<boolean>
  templateTarget: Ref<string>
  statement: Ref<string>
}

export function useRedisTemplates({ store, isRedis, templateTarget, statement }: Params) {
  const redisTemplateDefaults = {
    value: 'value',
    field: 'field',
    start: '0',
    stop: '20',
  }

  const templates = computed<Template[]>(() => {
    if (!store.current) return []
    if (isRedis.value) {
      return [
        { label: 'GET', statement: 'GET {{target}}' },
        { label: 'SET', statement: 'SET {{target}} {{value}}' },
        { label: 'HGET', statement: 'HGET {{target}} {{field}}' },
        { label: 'HGETALL', statement: 'HGETALL {{target}}' },
        { label: 'LRANGE', statement: 'LRANGE {{target}} {{start}} {{stop}}' },
        { label: 'SMEMBERS', statement: 'SMEMBERS {{target}}' },
        { label: 'ZRANGE', statement: 'ZRANGE {{target}} {{start}} {{stop}} WITHSCORES' },
        { label: 'XRANGE', statement: 'XRANGE {{target}} - + COUNT 20' },
        { label: 'TTL', statement: 'TTL {{target}}' },
        { label: 'DEL', statement: 'DEL {{target}}' },
      ]
    }
    return []
  })

  const applyTemplate = (tpl: Template) => {
    if (!store.current) return
    const context = {
      target: templateTarget.value || store.selectedEntity || '<target>',
      value: redisTemplateDefaults.value,
      field: redisTemplateDefaults.field,
      start: redisTemplateDefaults.start,
      stop: redisTemplateDefaults.stop,
    }
    statement.value = tpl.statement
      .replaceAll('{{target}}', context.target)
      .replaceAll('{{value}}', context.value)
      .replaceAll('{{field}}', context.field)
      .replaceAll('{{start}}', context.start)
      .replaceAll('{{stop}}', context.stop)
  }

  return { templates, applyTemplate }
}
