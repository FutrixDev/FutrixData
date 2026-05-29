export const EventsOn = () => () => {}
export const BrowserOpenURL = async (_url: string) => undefined
export const WindowSetDarkTheme = async () => undefined
export const WindowSetLightTheme = async () => undefined
export const WindowMinimise = async () => undefined
export const WindowToggleMaximise = async () => undefined
export const Quit = async () => undefined
export const Environment = async () => ({
  platform: 'darwin',
  arch: 'arm64',
  buildType: 'development',
})
