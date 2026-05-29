import { describe, expect, it } from 'vitest'

import { aiChatApi } from '@/services/api/aichat'

describe('aiChatApi mock visualization (vega_lite)', () => {
  it('returns vega_lite renderer after approving create_visualization', async () => {
    const conversationId = 'convo_viz_mock_1'

    await aiChatApi.aiChatTurn({
      conversationId,
      messages: [{ role: 'user', content: 'run select * from table_name' }],
    } as any)

    const ask = await aiChatApi.aiChatTurn({
      conversationId,
      messages: [{ role: 'user', content: 'visualize the result' }],
    } as any)

    expect(ask.approval?.kind).toBe('create_visualization')
    expect(ask.approval?.id).toBeTruthy()

    const approved = await aiChatApi.aiChatApprove({
      conversationId,
      approvalId: ask.approval!.id,
      decision: 'approve',
    } as any)

    expect((approved.effects as any)?.visualization?.renderer).toBe('vega_lite')
    expect((approved.effects as any)?.visualization?.spec).toBeTruthy()
  })
})
