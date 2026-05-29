import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import StartupRecoveryView from '@/components/startup/StartupRecoveryView.vue'
import { resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'
import { api } from '@/services/api'

describe('StartupRecoveryView', () => {
  beforeEach(() => {
    resetAppI18nForTest()
    setAppLocale('en')
    vi.restoreAllMocks()
  })

  it('renders classified recovery actions without loading the normal shell', async () => {
    const status = {
      state: 'failed',
      error: {
        reason: 'key_mismatch',
        message: 'The local encrypted data could not be opened with this device key.',
        dataPath: '/tmp/FutrixData/datasources.json',
        actions: ['retry', 'open_logs', 'move_aside_and_restart'],
      },
    }
    vi.spyOn(api, 'startupRecoveryRetry').mockResolvedValue({ state: 'failed', error: status.error } as any)
    vi.spyOn(api, 'startupRecoveryOpenLogs').mockResolvedValue(undefined)

    const wrapper = mount(StartupRecoveryView, {
      props: { status },
    })

    expect(wrapper.text()).toContain(tApp('startupRecovery.title'))
    expect(wrapper.text()).toContain(tApp('startupRecovery.reason.key_mismatch'))
    expect(wrapper.text()).toContain('/tmp/FutrixData/datasources.json')

    await wrapper.get('[data-testid="startup-recovery-retry"]').trigger('click')
    await flushPromises()
    expect(api.startupRecoveryRetry).toHaveBeenCalledTimes(1)
  })

  it('requires explicit confirmation before moving old encrypted data aside', async () => {
    const status = {
      state: 'failed',
      error: {
        reason: 'corrupt_file',
        message: 'The local encrypted data appears damaged.',
        dataPath: '/tmp/FutrixData/datasources.json',
        actions: ['move_aside_and_restart'],
      },
    }
    const moveAside = vi.spyOn(api, 'startupRecoveryMoveAsideAndRestart').mockResolvedValue({
      state: 'ready',
      movedAside: {
        retentionDir: '/tmp/FutrixData-recovered-20260502T100000Z',
      },
    } as any)

    const wrapper = mount(StartupRecoveryView, {
      props: { status },
    })

    await wrapper.get('[data-testid="startup-recovery-move-aside"]').trigger('click')
    expect(moveAside).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="startup-recovery-confirm"]').setValue(true)
    await wrapper.get('[data-testid="startup-recovery-move-aside"]').trigger('click')
    await flushPromises()

    expect(moveAside).toHaveBeenCalledWith(true)
  })
})
