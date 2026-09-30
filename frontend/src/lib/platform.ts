// Keyboard labels follow the OS: ⌘ on macOS, Ctrl elsewhere.
export const isMac = /Mac|iPhone|iPad/.test(navigator.userAgent)
export const mod = isMac ? '⌘' : 'Ctrl'
export const shortcut = (key: string) => (isMac ? `⌘${key}` : `Ctrl ${key}`)
