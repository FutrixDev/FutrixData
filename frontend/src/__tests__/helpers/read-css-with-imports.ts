import fs from 'node:fs'
import path from 'node:path'

const importRe = /^\s*@import\s+(?:url\()?["']([^"']+)["']\)?\s*;\s*$/gm

const shouldInlineImport = (specifier: string) => {
  if (!specifier) return false
  if (specifier === 'tailwindcss') return false
  if (specifier.startsWith('http://') || specifier.startsWith('https://')) return false
  if (specifier.startsWith('data:')) return false
  return specifier.startsWith('.') || specifier.startsWith('/')
}

export const readCssWithImports = (entryPath: string): string => {
  const visited = new Set<string>()

  const readFile = (filePath: string): string => {
    const absolutePath = path.resolve(filePath)
    if (visited.has(absolutePath)) return ''
    visited.add(absolutePath)

    const dir = path.dirname(absolutePath)
    const content = fs.readFileSync(absolutePath, 'utf-8')

    return content.replace(importRe, (fullMatch, specifier: string) => {
      if (!shouldInlineImport(specifier)) return ''
      const resolved = specifier.startsWith('/')
        ? path.resolve(dir, '.' + specifier)
        : path.resolve(dir, specifier)
      if (!fs.existsSync(resolved)) {
        return fullMatch
      }
      return readFile(resolved)
    })
  }

  return readFile(entryPath)
}
