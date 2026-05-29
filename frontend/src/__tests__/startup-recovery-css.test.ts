import { describe, expect, it } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'

const sourcePath = path.resolve(__dirname, '../components/startup/StartupRecoveryView.vue')
const source = fs.readFileSync(sourcePath, 'utf8')

describe('startup recovery css', () => {
  it('keeps recovery actions and confirmation controls stable across narrow layouts', () => {
    const actionButtonRule = source.match(/\.startup-recovery__actions\s+\.btn\s*\{[\s\S]*?\}/)?.[0] ?? ''
    expect(actionButtonRule).toMatch(/flex:\s*0\s+0\s+auto/i)
    expect(actionButtonRule).toMatch(/white-space:\s*nowrap/i)

    const confirmInputRule = source.match(/\.startup-recovery__confirm\s+input\s*\{[\s\S]*?\}/)?.[0] ?? ''
    expect(confirmInputRule).toMatch(/width:\s*16px/i)
    expect(confirmInputRule).toMatch(/height:\s*16px/i)
    expect(confirmInputRule).toMatch(/flex:\s*0\s+0\s+auto/i)

    expect(source).toContain('@media (max-width: 760px)')
    expect(source).toMatch(/\.startup-recovery__panel\s*\{[\s\S]*?grid-template-columns:\s*1fr/i)
  })
})
