/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

/*!
 * DeepSeek silhouette from @lobehub/icons, MIT License
 * Copyright (c) 2023 LobeHub
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

/** DeepSeek 鲸鱼轮廓，复用项目已安装的 `@lobehub/icons` 路径。 */
const DEEPSEEK_WHALE_PATH =
  'M23.748 4.482c-.254-.124-.364.113-.512.234-.051.039-.094.09-.137.136-.372.397-.806.657-1.373.626-.829-.046-1.537.214-2.163.848-.133-.782-.575-1.248-1.247-1.548-.352-.156-.708-.311-.955-.65-.172-.241-.219-.51-.305-.774-.055-.16-.11-.323-.293-.35-.2-.031-.278.136-.356.276-.313.572-.434 1.202-.422 1.84.027 1.436.633 2.58 1.838 3.393.137.093.172.187.129.323-.082.28-.18.552-.266.833-.055.179-.137.217-.329.14a5.526 5.526 0 01-1.736-1.18c-.857-.828-1.631-1.742-2.597-2.458a11.365 11.365 0 00-.689-.471c-.985-.957.13-1.743.388-1.836.27-.098.093-.432-.779-.428-.872.004-1.67.295-2.687.684a3.055 3.055 0 01-.465.137 9.597 9.597 0 00-2.883-.102c-1.885.21-3.39 1.102-4.497 2.623C.082 8.606-.231 10.684.152 12.85c.403 2.284 1.569 4.175 3.36 5.653 1.858 1.533 3.997 2.284 6.438 2.14 1.482-.085 3.133-.284 4.994-1.86.47.234.962.327 1.78.397.63.059 1.236-.03 1.705-.128.735-.156.684-.837.419-.961-2.155-1.004-1.682-.595-2.113-.926 1.096-1.296 2.746-2.642 3.392-7.003.05-.347.007-.565 0-.845-.004-.17.035-.237.23-.256a4.173 4.173 0 001.545-.475c1.396-.763 1.96-2.015 2.093-3.517.02-.23-.004-.467-.247-.588zM11.581 18c-2.089-1.642-3.102-2.183-3.52-2.16-.392.024-.321.471-.235.763.09.288.207.486.371.739.114.167.192.416-.113.603-.673.416-1.842-.14-1.897-.167-1.361-.802-2.5-1.86-3.301-3.307-.774-1.393-1.224-2.887-1.298-4.482-.02-.386.093-.522.477-.592a4.696 4.696 0 011.529-.039c2.132.312 3.946 1.265 5.468 2.774.868.86 1.525 1.887 2.202 2.891.72 1.066 1.494 2.082 2.48 2.914.348.292.625.514.891.677-.802.09-2.14.11-3.054-.614zm1-6.44a.306.306 0 01.415-.287.302.302 0 01.2.288.306.306 0 01-.31.307.303.303 0 01-.304-.308zm3.11 1.596c-.2.081-.399.151-.59.16a1.245 1.245 0 01-.798-.254c-.274-.23-.47-.358-.552-.758a1.73 1.73 0 01.016-.588c.07-.327-.008-.537-.239-.727-.187-.156-.426-.199-.688-.199a.559.559 0 01-.254-.078c-.11-.054-.2-.19-.114-.358.028-.054.16-.186.192-.21.356-.202.767-.136 1.146.016.352.144.618.408 1.001.782.391.451.462.576.685.914.176.265.336.537.445.848.067.195-.019.354-.25.452z'

/** 由官方轮廓、动态尾鳍和分层胸鳍组成的缓存路径。 */
interface WhalePaths {
  bodyParticles: Path2D
  farFin: Path2D
  nearFin: Path2D
  tail: Path2D
  tailParticles: Path2D
  whale: Path2D
}

let cachedWhalePaths: WhalePaths | null = null

/** 创建稳定的轮廓内粒子，避免动画帧内分配与随机抖动。 */
function createParticlePath(
  count: number,
  seed: number,
  minX: number,
  maxX: number,
  minY: number,
  maxY: number
): Path2D {
  const path = new Path2D()
  let state = seed >>> 0

  for (let index = 0; index < count; index += 1) {
    state += 0x6d2b79f5
    let value = state
    value = Math.imul(value ^ (value >>> 15), value | 1)
    value ^= value + Math.imul(value ^ (value >>> 7), value | 61)
    const xRatio = ((value ^ (value >>> 14)) >>> 0) / 4294967296

    state += 0x6d2b79f5
    value = state
    value = Math.imul(value ^ (value >>> 15), value | 1)
    value ^= value + Math.imul(value ^ (value >>> 7), value | 61)
    const yRatio = ((value ^ (value >>> 14)) >>> 0) / 4294967296

    state += 0x6d2b79f5
    value = state
    value = Math.imul(value ^ (value >>> 15), value | 1)
    value ^= value + Math.imul(value ^ (value >>> 7), value | 61)
    const sizeRatio = ((value ^ (value >>> 14)) >>> 0) / 4294967296

    const x = minX + xRatio * (maxX - minX)
    const y = minY + yRatio * (maxY - minY)
    const radius = 0.055 + sizeRatio * 0.12
    path.moveTo(x + radius, y)
    path.arc(x, y, radius, 0, Math.PI * 2)
  }

  return path
}

/** 延迟创建 Path2D，测试或旧环境缺少矢量 API 时安全跳过装饰层。 */
function getWhalePaths(): WhalePaths | null {
  if (cachedWhalePaths) {
    return cachedWhalePaths
  }
  if (typeof Path2D === 'undefined') {
    return null
  }

  try {
    cachedWhalePaths = {
      bodyParticles: createParticlePath(30, 0x64656570, 0.5, 19, 3.2, 20.7),
      farFin: new Path2D(
        'M8.15 15.55C9.45 15.72 11.5 17.05 13.15 18.5C12.25 20.25 10.65 21.25 9.05 20.82C7.82 20.5 7.2 19.72 7.72 19.05C8.25 18.35 8.05 16.7 8.15 15.55Z'
      ),
      nearFin: new Path2D(
        'M14.65 16.15C16.2 16.5 18.65 17.7 20.15 18.45C20.82 18.8 20.52 19.45 19.75 19.72C17.85 20.35 15.7 19.78 14.05 18.42C13.52 17.98 13.72 16.62 14.65 16.15Z'
      ),
      tail: new Path2D(
        'M18.45 9.5C19.05 8.55 18.82 8.12 18.05 7.58C16.85 6.72 16.25 5.35 16.48 3.92C16.65 2.83 17.12 2.2 17.48 2.92C17.82 3.78 17.9 4.38 18.78 4.72C19.72 5.08 20.2 5.8 20.38 6.62C21.18 5.98 22.2 5.48 23.22 5.5C24.0 5.52 24.28 4.9 24.78 4.45C25.35 3.95 25.48 4.35 25.35 5.18C25.05 7.08 24.22 8.48 22.75 9.32C21.72 9.92 20.25 10.2 18.45 9.5Z'
      ),
      tailParticles: createParticlePath(10, 0x7768616c, 17, 25, 2.7, 10.1),
      whale: new Path2D(DEEPSEEK_WHALE_PATH),
    }
  } catch {
    return null
  }

  return cachedWhalePaths
}

/** 绘制下潜穿过粒子环、上浮后回游的鲸鱼；时间单位为秒，暂停由主场景管理。 */
export function drawDeepSeekWhale(
  context: CanvasRenderingContext2D,
  width: number,
  height: number,
  time: number
): void {
  if (
    typeof context.arc !== 'function' ||
    typeof context.bezierCurveTo !== 'function' ||
    typeof context.clip !== 'function' ||
    typeof context.createLinearGradient !== 'function' ||
    typeof context.fill !== 'function' ||
    typeof context.rotate !== 'function' ||
    typeof context.scale !== 'function' ||
    typeof context.translate !== 'function'
  ) {
    return
  }

  const paths = getWhalePaths()
  if (!paths) {
    return
  }

  const compact = width < 680
  const medium = width >= 680 && width < 1180
  let whaleWidth = Math.min(232, width * 0.165)
  if (compact) {
    whaleWidth = Math.min(132, width * 0.34)
  } else if (medium) {
    whaleWidth = Math.min(206, width * 0.21)
  }
  // 闭合轨迹依次经过环顶、右侧下潜、环心和左侧上浮，20 秒完成一圈。
  const phase = (((time + 1.2) % 20) / 20) * Math.PI * 2
  const diveProgress = (1 - Math.cos(phase)) * 0.5
  const horizontalRange = compact ? 0.22 : 0.27
  const centerX = width * (0.5 + Math.sin(phase) * horizontalRange)
  const centerY = height * (0.16 + diveProgress * 0.39)
  const velocityX = Math.cos(phase) * horizontalRange * width
  const velocityY = Math.sin(phase) * 0.195 * height
  // 转身时短暂呈侧影，避免镜像突变；环心内缩小并降低亮度，保留文字对比度。
  const facing = -Math.tanh(velocityX / (width * 0.065))
  const roll =
    Math.atan2(-velocityY, Math.abs(velocityX) + width * 0.055) * facing
  const depthScale = 1 - diveProgress * 0.32
  const whaleHeight = whaleWidth * 0.72 * depthScale
  const tailWag = Math.sin(time * 3.1) * (0.13 + diveProgress * 0.05)
  const depthAlpha = 1 - diveProgress * 0.65

  context.save()
  context.translate(centerX, centerY)
  context.rotate(roll)
  context.scale((whaleWidth * depthScale * facing) / 24, whaleHeight / 24)
  context.translate(-12, -12)
  context.globalCompositeOperation = 'lighter'
  context.globalAlpha = depthAlpha
  context.lineCap = 'round'
  context.lineJoin = 'round'

  for (let trail = 0; trail < 2; trail += 1) {
    const trailWave = Math.sin(time * 0.9 + trail * 1.7)
    context.beginPath()
    context.moveTo(22.4, 6.5 + trail * 0.75)
    context.bezierCurveTo(
      26.2,
      5.9 + trail * 0.72 + trailWave * 0.45,
      30.4,
      7.2 + trail * 0.54 - trailWave * 0.6,
      35.2,
      6.25 + trail * 0.64
    )
    context.globalAlpha = (0.24 - trail * 0.055) * depthAlpha
    context.strokeStyle = trail === 1 ? 'rgb(131 228 255)' : 'rgb(38 177 255)'
    context.lineWidth = 0.095 - trail * 0.014
    context.setLineDash([0.26 + trail * 0.08, 0.46 + trail * 0.12])
    context.stroke()
  }
  context.setLineDash([])

  context.globalAlpha = 0.22 * depthAlpha
  context.fillStyle = 'rgb(29 92 207)'
  context.fill(paths.farFin)

  const bodyGradient = context.createLinearGradient(1, 4, 21, 19)
  bodyGradient.addColorStop(0, 'rgb(123 245 255 / 0.42)')
  bodyGradient.addColorStop(0.48, 'rgb(39 174 255 / 0.34)')
  bodyGradient.addColorStop(1, 'rgb(77 107 254 / 0.24)')

  context.globalAlpha = 0.82 * depthAlpha
  context.fillStyle = bodyGradient
  context.fill(paths.whale)

  context.save()
  context.clip(paths.whale)
  context.globalAlpha = (0.5 + Math.sin(time * 0.72) * 0.08) * depthAlpha
  context.fillStyle = 'rgb(186 249 255)'
  context.fill(paths.bodyParticles)
  context.restore()

  context.save()
  context.translate(18.45, 9.5)
  context.rotate(tailWag)
  context.translate(-18.45, -9.5)
  const tailGradient = context.createLinearGradient(17, 3, 25, 10)
  tailGradient.addColorStop(0, 'rgb(82 222 255 / 0.38)')
  tailGradient.addColorStop(1, 'rgb(77 107 254 / 0.3)')
  context.globalAlpha = 0.9 * depthAlpha
  context.fillStyle = tailGradient
  context.fill(paths.tail)
  context.globalAlpha = 0.55 * depthAlpha
  context.strokeStyle = 'rgb(142 242 255)'
  context.lineWidth = 0.11
  context.stroke(paths.tail)
  context.clip(paths.tail)
  context.globalAlpha = (0.54 + Math.sin(time * 0.86 + 1.2) * 0.08) * depthAlpha
  context.fillStyle = 'rgb(207 252 255)'
  context.fill(paths.tailParticles)
  context.restore()

  context.globalAlpha = 0.28 * depthAlpha
  context.fillStyle = 'rgb(59 139 255)'
  context.fill(paths.nearFin)
  context.globalAlpha = 0.6 * depthAlpha
  context.strokeStyle = 'rgb(108 225 255)'
  context.lineWidth = 0.09
  context.stroke(paths.nearFin)

  context.globalAlpha = 0.25 * depthAlpha
  context.strokeStyle = 'rgb(81 179 255)'
  context.lineWidth = 0.08
  context.stroke(paths.whale)

  context.globalAlpha = 0.95 * depthAlpha
  context.fillStyle = 'rgb(2 18 37)'
  context.beginPath()
  context.arc(12.89, 11.56, 0.42, 0, Math.PI * 2)
  context.fill()
  context.globalAlpha = 0.95 * depthAlpha
  context.fillStyle = 'rgb(217 253 255)'
  context.beginPath()
  context.arc(12.78, 11.44, 0.13, 0, Math.PI * 2)
  context.fill()

  context.restore()
}
