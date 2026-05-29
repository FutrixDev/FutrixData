import fs from 'node:fs'
import path from 'node:path'

import { describe, expect, it } from 'vitest'

const componentPath = path.resolve(__dirname, '..', 'components', 'ConsoleMonacoEditor.vue')
const source = fs.readFileSync(componentPath, 'utf8')

describe('console monaco editor css regressions', () => {
  it('keeps monaco editor containers shrink-safe inside narrow sql parity layouts', () => {
    const root = source.match(/\.console-monaco-editor\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const viewport = source.match(/\.console-monaco-editor__viewport\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(root).toMatch(/min-width:\s*0/i)
    expect(root).toMatch(/overflow:\s*hidden/i)
    expect(viewport).toMatch(/min-width:\s*0/i)
    expect(viewport).toMatch(/overflow:\s*hidden/i)
  })
})
