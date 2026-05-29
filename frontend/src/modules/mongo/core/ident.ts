export function isValidMongoIdent(name: string) {
  return /^[A-Za-z_$][A-Za-z0-9_$]*$/.test(name || '')
}

export function formatMongoFieldKey(name: string) {
  if (isValidMongoIdent(name)) return name
  return `\"${name}\"`
}
