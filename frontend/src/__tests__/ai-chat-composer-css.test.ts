import path from 'node:path'

import { describe, expect, it } from 'vitest'

import { readCssWithImports } from './helpers/read-css-with-imports'

const loadStyleCss = () => {
  const filePath = path.resolve(__dirname, '..', 'style.css')
  return readCssWithImports(filePath)
}

describe('AI chat composer CSS', () => {
  it('uses a boxed composer with growable textarea', () => {
    const css = loadStyleCss()

    expect(css).toContain('.ai-composer-box {')
    expect(css).toContain('border-radius: 14px;')
    expect(css).toContain('.ai-composer-input-area {')
    expect(css).toContain('resize: none;')
    expect(css).toContain('max-height: 120px;')
  })

  it('keeps model selector and actions aligned on composer footer', () => {
    const css = loadStyleCss()

    expect(css).toContain('.ai-composer-toolbar {')
    expect(css).toContain('justify-content: space-between;')
    expect(css).toContain('.ai-composer-actions {')
    expect(css).toContain('.ai-voice-btn {')
    expect(css).toContain('.ai-send-circle-btn {')
  })

  it('uses a sparkle icon for the provider trigger', () => {
    const css = loadStyleCss()

    expect(css).toContain('.ai-composer-icon {')
    expect(css).toContain('border-radius: 50%;')
    expect(css).toContain('.ai-composer-icon::before {')
    expect(css).toContain('mask-image: url(')
    expect(css).toContain('linear-gradient')
  })
})
