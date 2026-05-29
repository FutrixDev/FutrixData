// @vitest-environment node

import fs from 'node:fs'
import path from 'node:path'

import { afterEach, describe, expect, it, vi } from 'vitest'

import { resolveWailsAliasDir } from '../../vite.wails'

const frontendRoot = path.resolve(__dirname, '../..')

describe('vite @wailsjs alias', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('uses the test stubs while running vitest', () => {
    expect(resolveWailsAliasDir(frontendRoot, true)).toBe(path.resolve(frontendRoot, 'src/test/wailsjs'))
  })

  it('uses generated bindings outside vitest when they exist', () => {
    vi.spyOn(fs, 'existsSync').mockImplementation((filePath) => (
      String(filePath).includes(path.join('wailsjs', 'go', 'main', 'App.js'))
      || String(filePath).includes(path.join('wailsjs', 'go', 'models.ts'))
      || String(filePath).includes(path.join('wailsjs', 'runtime', 'runtime.js'))
    ))

    expect(resolveWailsAliasDir(frontendRoot, false)).toBe(path.resolve(frontendRoot, 'wailsjs'))
  })

  it('falls back to test stubs outside vitest when generated bindings are missing', () => {
    vi.spyOn(fs, 'existsSync').mockReturnValue(false)

    expect(resolveWailsAliasDir(frontendRoot, false)).toBe(path.resolve(frontendRoot, 'src/test/wailsjs'))
  })

  it('falls back to test stubs when the generated bindings directory is incomplete', () => {
    vi.spyOn(fs, 'existsSync').mockImplementation((filePath) => (
      String(filePath).includes(path.join('wailsjs', 'go', 'main', 'App.js'))
    ))

    expect(resolveWailsAliasDir(frontendRoot, false)).toBe(path.resolve(frontendRoot, 'src/test/wailsjs'))
  })
})
