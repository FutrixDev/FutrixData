import { describe, it, expect } from 'vitest'
import { readFileSync } from 'fs'
import { resolve } from 'path'

/**
 * CSS regression tests for sensitivity pages.
 * Ensures no hard-coded max-width constraints re-appear on top-level containers.
 */

const viewsDir = resolve(__dirname, '../views')

describe('SensitivityListView responsive layout', () => {
  const src = readFileSync(resolve(viewsDir, 'SensitivityListView.vue'), 'utf-8')
  const style = src.match(/<style[^>]*>([\s\S]*?)<\/style>/)?.[1] ?? ''

  it('should not have a max-width on .sensitivity-list-view', () => {
    // Extract the .sensitivity-list-view rule block
    const ruleMatch = style.match(/\.sensitivity-list-view\s*\{([^}]*)\}/)
    expect(ruleMatch).toBeTruthy()
    const rule = ruleMatch![1]

    expect(rule).not.toMatch(/max-width\s*:/)
  })

  it('keeps sensitivity configuration icon controls at 32px tap targets', () => {
    const colorTrigger = style.match(/\.sens-level__color-trigger\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const colorDot = style.match(/\.sens-level__color-dot\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const deleteButton = style.match(/\.sens-level__delete\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const addTag = style.match(/\.sens-level__tag-add\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(colorTrigger).toMatch(/width:\s*32px/i)
    expect(colorTrigger).toMatch(/height:\s*32px/i)
    expect(colorDot).toMatch(/width:\s*32px/i)
    expect(colorDot).toMatch(/height:\s*32px/i)
    expect(deleteButton).toMatch(/width:\s*32px/i)
    expect(deleteButton).toMatch(/height:\s*32px/i)
    expect(addTag).toMatch(/width:\s*32px/i)
    expect(addTag).toMatch(/height:\s*32px/i)
  })
})

describe('SensitivityView responsive layout', () => {
  const src = readFileSync(resolve(viewsDir, 'SensitivityView.vue'), 'utf-8')

  it('should not have max-w-* Tailwind constraint on root section', () => {
    // Find the root <section> tag in the template
    const sectionMatch = src.match(/<section\s[^>]*class="([^"]*)"/)
    expect(sectionMatch).toBeTruthy()
    const classes = sectionMatch![1]

    expect(classes).not.toMatch(/max-w-/)
  })

  it('keeps the scan action at a 32px tap target', () => {
    expect(src).toContain('min-h-[32px] px-4 py-2 rounded-lg bg-primary')
  })

  it('keeps field override actions at 32px tap targets', () => {
    const matches = src.match(/inline-flex min-h-\[32px\] items-center text-xs text-primary hover:underline/g) || []

    expect(matches.length).toBeGreaterThanOrEqual(2)
  })
})
