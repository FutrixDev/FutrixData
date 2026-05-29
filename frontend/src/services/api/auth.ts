import {
  CompleteAuthLogin,
  CurrentAuth,
  EnsureAuthenticated,
  ListAuthDevices,
  LogoutAuth,
  PollAuthLogin,
  RemoveAuthDevice,
  StartAuthLogin,
} from '@wailsjs/go/main/App'

import type { AuthDeviceList, AuthLoginPoll, AuthLoginStart, AuthState } from '@/types'

import { cloneJson, newId, withMock } from './core'

let mockAuthState: AuthState = {
  deviceId: 'device_mock',
  session: null,
  pendingLogin: null,
  trial: {
    startedAt: Math.floor(Date.now() / 1000),
    expiresAt: Math.floor(Date.now() / 1000) + 30 * 24 * 60 * 60,
  },
}

const mockEnsureAuthenticated = async (): Promise<AuthState> => cloneJson(mockAuthState)
const mockCurrentAuth = async (): Promise<AuthState> => cloneJson(mockAuthState)

const mockStartAuthLogin = async (_input: { noBrowser?: boolean }): Promise<AuthLoginStart> => {
  const sessionId = newId('auth')
  const loginUrl = `https://futrixdata.com/app?session_id=${sessionId}`
  mockAuthState = {
    ...mockAuthState,
    pendingLogin: {
      sessionId,
      codeVerifier: 'mock_verifier',
      loginUrl,
    },
  }
  return cloneJson({ loginUrl, sessionId })
}

const mockPollAuthLogin = async (): Promise<AuthLoginPoll> => cloneJson({ status: 'pending' })

const mockCompleteAuthLogin = async (_code: string): Promise<AuthState> => {
  mockAuthState = {
    ...mockAuthState,
    pendingLogin: null,
    session: {
      accessToken: 'access_mock',
      refreshToken: 'refresh_mock',
      expiresAt: Date.now() + 15 * 60 * 1000,
      user: {
        id: 'user_mock',
        email: 'user@example.com',
        displayName: 'Mock User',
        avatarUrl: '',
      },
      license: {
        plan: 'free',
        status: 'active',
        expiresAt: 0,
      },
    },
  }
  return cloneJson(mockAuthState)
}

const mockLogoutAuth = async (): Promise<AuthState> => {
  mockAuthState = {
    ...mockAuthState,
    session: null,
    pendingLogin: null,
    trial: mockAuthState.trial,
  }
  return cloneJson(mockAuthState)
}

const mockListAuthDevices = async (): Promise<AuthDeviceList> => {
  return cloneJson({
    devices: [
      {
        deviceId: mockAuthState.deviceId,
        deviceName: 'Mock Device',
        platform: 'macos',
        lastActiveAt: Date.now(),
        createdAt: Date.now(),
      },
    ],
    limit: 1,
    plan: mockAuthState.session?.license.plan || 'free',
  })
}

const mockRemoveAuthDevice = async (_deviceID: string): Promise<AuthDeviceList> => {
  return cloneJson({
    devices: [],
    limit: 1,
    plan: mockAuthState.session?.license.plan || 'free',
  })
}

export const authApi = {
  currentAuth: () => withMock(() => CurrentAuth(), mockCurrentAuth),
  ensureAuthenticated: () => withMock(() => EnsureAuthenticated(), mockEnsureAuthenticated),
  startAuthLogin: (input: { noBrowser?: boolean } = {}) =>
    withMock(() => StartAuthLogin(input), () => mockStartAuthLogin(input)),
  pollAuthLogin: () => withMock(() => PollAuthLogin(), mockPollAuthLogin),
  completeAuthLogin: (code: string) =>
    withMock(() => CompleteAuthLogin(code), () => mockCompleteAuthLogin(code)),
  logoutAuth: () => withMock(() => LogoutAuth(), mockLogoutAuth),
  listAuthDevices: () => withMock(() => ListAuthDevices(), mockListAuthDevices),
  removeAuthDevice: (deviceID: string) =>
    withMock(() => RemoveAuthDevice(deviceID), () => mockRemoveAuthDevice(deviceID)),
}
