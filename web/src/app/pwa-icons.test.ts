import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import sharp from 'sharp'
import { describe, expect, it } from 'vitest'

const publicDir = resolve(process.cwd(), 'public')

describe('PWA icons', () => {
  it('publishes installable 192px and 512px PNG icons', () => {
    const manifest = JSON.parse(readFileSync(resolve(publicDir, 'manifest.json'), 'utf8'))
    expect(manifest.icons).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ src: '/icon-192.png', sizes: '192x192', type: 'image/png' }),
        expect.objectContaining({ src: '/icon-512.png', sizes: '512x512', type: 'image/png' }),
      ])
    )
  })

  it('ships each raster asset at its declared dimensions', () => {
    expect({
      icon192: pngDimensions('icon-192.png'),
      icon512: pngDimensions('icon-512.png'),
      appleTouch: pngDimensions('apple-touch-icon.png'),
    }).toEqual({
      icon192: [192, 192],
      icon512: [512, 512],
      appleTouch: [180, 180],
    })
  })

  it('preserves the lime app-mark background when rasterized', async () => {
    const { data, info } = await sharp(resolve(publicDir, 'icon-512.png'))
      .ensureAlpha()
      .raw()
      .toBuffer({ resolveWithObject: true })
    const offset = (32 * info.width + 256) * info.channels

    expect([...data.subarray(offset, offset + 4)]).toEqual([212, 255, 58, 255])
  })
})

function pngDimensions(filename: string): [number, number] {
  const png = readFileSync(resolve(publicDir, filename))
  return [png.readUInt32BE(16), png.readUInt32BE(20)]
}
