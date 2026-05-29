import path from 'node:path'
import { describe, expect, it } from 'vitest'

import { readCssWithImports } from './helpers/read-css-with-imports'

const css = readCssWithImports(path.resolve(__dirname, '..', 'style.css'))

describe('AI chat CSS', () => {
  it('defines ai sidebar and context chip overlay', () => {
    expect(css).toMatch(/\.ai-sidebar[\s\S]*?\{[\s\S]*?--ai-rail/)
    expect(css).toMatch(/--ai-ivory:\s*#f9faef/i)
    expect(css).toMatch(/--ai-paper:\s*#f3f4e9/i)
    expect(css).toMatch(/--ai-divider:\s*#c5c8ba/i)
    expect(css).toMatch(/--ai-ink:\s*#1a1c16/i)
    expect(css).toMatch(/--ai-ink-muted:\s*#44483d/i)
    expect(css).toMatch(/--ai-sage:\s*#4c662b/i)
    expect(css).toMatch(/--ai-amber:\s*#b1d18a/i)
    expect(css).toMatch(/\.ai-quick-prompt[\s\S]*?--ai-ivory:\s*#f9faef/i)
    expect(css).toMatch(/\.ai-context-chip::after[\s\S]*?content:/)
  })

  it('styles ai history tabs and compact model selector', () => {
    const sidebar = css.match(/\.ai-sidebar[\s\S]*?\}/)?.[0] ?? ''
    expect(sidebar).toMatch(/padding-bottom:\s*48px/i)

    const historyStrip = css.match(/\.ai-history-strip[\s\S]*?\}/)?.[0] ?? ''
    expect(historyStrip).toMatch(/background:\s*transparent/i)
    expect(historyStrip).toMatch(/border:\s*none/i)

    const historyTab = css.match(/\.ai-history-tab[\s\S]*?\}/)?.[0] ?? ''
    expect(historyTab).toMatch(/border-radius:\s*12px/i)

    const modelTrigger = css.match(/\.ai-model-trigger[\s\S]*?\}/)?.[0] ?? ''
    expect(modelTrigger).toMatch(/border:\s*none/i)
    expect(modelTrigger).toMatch(/border-radius:\s*8px/i)

    const modelSelect = css.match(/\.ai-model-select[\s\S]*?\}/)?.[0] ?? ''
    expect(modelSelect).toMatch(/max-width:\s*calc\(100%\s*-\s*90px\)/i)
  })

  it('adds a glass treatment to the ai toggle button', () => {
    const aiToggle = css.match(/\.btn\.ai-toggle[\s\S]*?\}/)?.[0] ?? ''
    expect(aiToggle).toMatch(/background:\s*linear-gradient/i)
    expect(aiToggle).toMatch(/backdrop-filter:\s*blur/i)
  })

  it('keeps ai sidebar icon buttons at 32px click targets even inside narrow flex rows', () => {
    const iconButton = css.match(/\.ai-icon-btn\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(iconButton).toMatch(/width:\s*32px/i)
    expect(iconButton).toMatch(/min-width:\s*32px/i)
    expect(iconButton).toMatch(/height:\s*32px/i)
    expect(iconButton).toMatch(/min-height:\s*32px/i)
    expect(iconButton).toMatch(/flex:\s*0\s+0\s+32px/i)
  })

  it('sizes the ai toggle as a 32px icon button without inherited label padding', () => {
    const aiToggle = css.match(/\.btn\.ai-toggle[\s\S]*?\}/)?.[0] ?? ''

    expect(aiToggle).toMatch(/width:\s*32px/i)
    expect(aiToggle).toMatch(/height:\s*32px/i)
    expect(aiToggle).toMatch(/min-height:\s*32px/i)
    expect(aiToggle).toMatch(/padding:\s*0/i)
  })
})
