import type { Page } from '@playwright/test'

// Component mocks need a valid frame, not a capture of an unpainted about:blank
// target. The actual owner API/Broker/runtime profile still uses real frames.
export async function browserFrameJPEG(page: Page): Promise<string> {
  return page.evaluate(() => {
    const canvas = document.createElement('canvas')
    canvas.width = 1280
    canvas.height = 720
    const context = canvas.getContext('2d')
    if (!context) throw new Error('Synthetic browser frame unavailable')
    context.fillStyle = '#e2e8f0'
    context.fillRect(0, 0, canvas.width, canvas.height)
    const prefix = 'data:image/jpeg;base64,'
    const encoded = canvas.toDataURL('image/jpeg')
    if (!encoded.startsWith(prefix)) throw new Error('Synthetic JPEG unavailable')
    return encoded.slice(prefix.length)
  })
}
