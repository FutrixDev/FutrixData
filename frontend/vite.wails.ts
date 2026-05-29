import fs from 'node:fs'
import path from 'node:path'

const requiredGeneratedBindings = [
  'go/main/App.js',
  'go/models.ts',
  'runtime/runtime.js',
]

export function resolveWailsAliasDir(rootDir: string, isVitest: boolean) {
  if (isVitest) {
    return path.resolve(rootDir, './src/test/wailsjs')
  }

  const generatedDir = path.resolve(rootDir, './wailsjs')
  if (hasCompleteGeneratedBindings(generatedDir)) {
    return generatedDir
  }

  return path.resolve(rootDir, './src/test/wailsjs')
}

function hasCompleteGeneratedBindings(generatedDir: string) {
  return requiredGeneratedBindings.every((relativePath) => (
    fs.existsSync(path.join(generatedDir, relativePath))
  ))
}
