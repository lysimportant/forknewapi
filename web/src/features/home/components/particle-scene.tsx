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
import { useEffect, useRef, type ReactElement } from 'react'

/** 完整圆周弧度。 */
const TAU = Math.PI * 2
/** 粒子贴图使用的青蓝与冰白 RGB 色板。 */
const PARTICLE_RGB = [
  [26, 221, 255],
  [38, 116, 255],
  [205, 250, 255],
  [83, 169, 255],
] as const
/** Canvas 降级绘制和轨道线使用的 CSS 色板。 */
const PARTICLE_COLORS = [
  'rgb(26 221 255)',
  'rgb(38 116 255)',
  'rgb(205 250 255)',
  'rgb(83 169 255)',
] as const

/** 环体粒子的固定世界坐标、流速和外观参数。 */
interface TorusParticle {
  alpha: number
  drift: number
  hue: number
  majorRadius: number
  phase: number
  size: number
  tubeRadius: number
  u: number
  v: number
}

/** 外围星尘的归一化位置、视差深度和闪烁参数。 */
interface StarParticle {
  alpha: number
  depth: number
  driftX: number
  driftY: number
  hue: number
  phase: number
  size: number
  x: number
  y: number
}

/** 底部透视波面的固定网格坐标和外观参数。 */
interface WaveParticle {
  column: number
  depth: number
  hue: number
  phase: number
  row: number
  size: number
  x: number
}

/** 每帧复用的二维投影结果。 */
interface ProjectedParticle {
  alpha: number
  depth: number
  hue: number
  scale: number
  x: number
  y: number
}

/** 当前密度档位下的全部粒子源数据和投影缓存。 */
interface ParticleField {
  projectedTorus: ProjectedParticle[]
  projectedWave: ProjectedParticle[]
  stars: StarParticle[]
  torus: TorusParticle[]
  wave: WaveParticle[]
  waveColumns: number
  waveRows: number
}

/** 以 CSS 像素表示的画布尺寸、中心与世界坐标缩放。 */
interface Viewport {
  centerX: number
  centerY: number
  height: number
  width: number
  xScale: number
  yScale: number
}

/** 一帧内复用的三轴旋转三角值。 */
interface ProjectionView extends Viewport {
  cosPitch: number
  cosRoll: number
  cosYaw: number
  sinPitch: number
  sinRoll: number
  sinYaw: number
}

/** 指针位置及平滑后的视角偏转状态。 */
interface PointerState {
  inside: boolean
  pitch: number
  targetPitch: number
  targetYaw: number
  x: number
  y: number
  yaw: number
}

/** 粒子场对 React 暴露的最小生命周期控制面。 */
interface ParticleSceneController {
  /** 释放动画帧、监听器和观察器。 */
  destroy: () => void
  /** 切换手动暂停；暂停时保留当前静态帧。 */
  setPaused: (paused: boolean) => void
}

/** 创建可复现的伪随机数序列，确保每次重绘保持稳定构图。 */
function createSeededRandom(seed: number): () => number {
  let state = seed >>> 0
  return () => {
    state += 0x6d2b79f5
    let value = state
    value = Math.imul(value ^ (value >>> 15), value | 1)
    value ^= value + Math.imul(value ^ (value >>> 7), value | 61)
    return ((value ^ (value >>> 14)) >>> 0) / 4294967296
  }
}

/** 按当前宽度生成分层环体、底部波面与外围星尘数据。 */
function createParticleField(width: number): ParticleField {
  const random = createSeededRandom(0x6d616e73)
  let torusCount = 3200
  let starCount = 280
  let waveColumns = 44
  let waveRows = 15

  if (width < 680) {
    torusCount = 1450
    starCount = 120
    waveColumns = 28
    waveRows = 10
  } else if (width < 1024) {
    torusCount = 2250
    starCount = 190
    waveColumns = 36
    waveRows = 13
  }

  const torus: TorusParticle[] = []
  for (let index = 0; index < torusCount; index += 1) {
    const layer = index % 7
    const u = TAU * ((index * 0.61803398875 + random() * 0.08) % 1)
    const v = TAU * ((index * 0.75487766625 + random() * 0.14) % 1)
    const highlight = random()
    let hue = index % 5 === 0 ? 3 : 0
    if (highlight > 0.91) {
      hue = 2
    } else if (highlight < 0.2) {
      hue = 1
    }

    torus.push({
      alpha: 0.38 + random() * 0.55,
      drift: 0.72 + random() * 0.72,
      hue,
      majorRadius: 1.6 + layer * 0.045 + (random() - 0.5) * 0.08,
      phase: random() * TAU,
      size: 0.48 + random() * (hue === 2 ? 1.18 : 0.72),
      tubeRadius: 0.4 + (layer % 3) * 0.065 + random() * 0.08,
      u,
      v,
    })
  }

  const stars: StarParticle[] = []
  for (let index = 0; index < starCount; index += 1) {
    let x = random()
    let y = 0.04 + random() * 0.86
    let attempts = 0
    while (
      ((x - 0.5) / 0.26) ** 2 + ((y - 0.48) / 0.2) ** 2 < 1.35 &&
      attempts < 8
    ) {
      x = random()
      y = 0.04 + random() * 0.86
      attempts += 1
    }

    const hueRoll = random()
    let hue = 0
    if (hueRoll > 0.82) {
      hue = 2
    } else if (hueRoll > 0.55) {
      hue = 3
    }

    stars.push({
      alpha: 0.16 + random() * 0.46,
      depth: random(),
      driftX: (random() - 0.5) * 0.006,
      driftY: (random() - 0.5) * 0.004,
      hue,
      phase: random() * TAU,
      size: 0.35 + random() * 0.85,
      x,
      y,
    })
  }

  const wave: WaveParticle[] = []
  for (let row = 0; row < waveRows; row += 1) {
    const depth = (row + 0.45) / waveRows
    for (let column = 0; column < waveColumns; column += 1) {
      const normalizedX = (column / (waveColumns - 1)) * 2 - 1
      let hue = 0
      if ((row + column) % 7 === 0) {
        hue = 2
      } else if (row % 3 === 0) {
        hue = 3
      }
      wave.push({
        column,
        depth,
        hue,
        phase: random() * 0.45,
        row,
        size: 0.42 + random() * 0.46,
        x: normalizedX,
      })
    }
  }

  return {
    projectedTorus: torus.map(() => ({
      alpha: 0,
      depth: 0,
      hue: 0,
      scale: 1,
      x: 0,
      y: 0,
    })),
    projectedWave: wave.map(() => ({
      alpha: 0,
      depth: 0,
      hue: 0,
      scale: 1,
      x: 0,
      y: 0,
    })),
    stars,
    torus,
    wave,
    waveColumns,
    waveRows,
  }
}

/** 预渲染带柔和光晕的粒子贴图，避免逐点创建渐变。 */
function createParticleSprites(): Array<HTMLCanvasElement | null> {
  return PARTICLE_RGB.map((rgb) => {
    const sprite = document.createElement('canvas')
    const size = 48
    sprite.width = size
    sprite.height = size
    const context = sprite.getContext('2d')
    if (!context) {
      return null
    }

    const center = size / 2
    const gradient = context.createRadialGradient(
      center,
      center,
      0,
      center,
      center,
      center
    )
    gradient.addColorStop(0, 'rgb(255 255 255 / 1)')
    gradient.addColorStop(0.12, `rgb(${rgb[0]} ${rgb[1]} ${rgb[2]} / 0.98)`)
    gradient.addColorStop(0.38, `rgb(${rgb[0]} ${rgb[1]} ${rgb[2]} / 0.36)`)
    gradient.addColorStop(1, `rgb(${rgb[0]} ${rgb[1]} ${rgb[2]} / 0)`)
    context.fillStyle = gradient
    context.fillRect(0, 0, size, size)
    return sprite
  })
}

/** 生成一帧共用的旋转与透视参数。 */
function createProjectionView(
  viewport: Viewport,
  pointer: PointerState,
  time: number
): ProjectionView {
  const pitch = 0.38 + pointer.pitch + Math.sin(time * 0.13) * 0.025
  const yaw = pointer.yaw + Math.cos(time * 0.11) * 0.035
  const roll = time * 0.035 + Math.sin(time * 0.17) * 0.018

  return {
    ...viewport,
    cosPitch: Math.cos(pitch),
    cosRoll: Math.cos(roll),
    cosYaw: Math.cos(yaw),
    sinPitch: Math.sin(pitch),
    sinRoll: Math.sin(roll),
    sinYaw: Math.sin(yaw),
  }
}

/** 将三维世界坐标投影到画布，并把结果写入复用对象。 */
function projectWorldPoint(
  x: number,
  y: number,
  z: number,
  view: ProjectionView,
  output: ProjectedParticle
): void {
  const rolledX = x * view.cosRoll - y * view.sinRoll
  const rolledY = x * view.sinRoll + y * view.cosRoll
  const pitchedY = rolledY * view.cosPitch - z * view.sinPitch
  const pitchedZ = rolledY * view.sinPitch + z * view.cosPitch
  const rotatedX = rolledX * view.cosYaw + pitchedZ * view.sinYaw
  const rotatedZ = -rolledX * view.sinYaw + pitchedZ * view.cosYaw
  const perspective = 7.2 / Math.max(3.6, 7.2 - rotatedZ)

  output.x = view.centerX + rotatedX * view.xScale * perspective
  output.y = view.centerY + pitchedY * view.yScale * perspective
  output.depth = Math.max(0, Math.min(1, (rotatedZ + 3.1) / 6.2))
  output.scale = perspective * (0.72 + output.depth * 0.5)
}

/** 为指针附近的前景粒子施加有限排斥，保持交互轻微且稳定。 */
function repelProjectedPoint(
  point: ProjectedParticle,
  pointer: PointerState,
  strength: number
): void {
  if (!pointer.inside) {
    return
  }

  const deltaX = point.x - pointer.x
  const deltaY = point.y - pointer.y
  const distanceSquared = deltaX * deltaX + deltaY * deltaY
  const radius = 86
  if (distanceSquared <= 1 || distanceSquared >= radius * radius) {
    return
  }

  const distance = Math.sqrt(distanceSquared)
  const force = ((radius - distance) / radius) ** 2 * strength
  point.x += (deltaX / distance) * force
  point.y += (deltaY / distance) * force
}

/** 绘制一个预渲染光点，贴图不可用时退化为纯色小点。 */
function drawParticle(
  context: CanvasRenderingContext2D,
  sprites: Array<HTMLCanvasElement | null>,
  point: ProjectedParticle,
  diameter: number
): void {
  if (point.alpha <= 0.003) {
    return
  }

  const sprite = sprites[point.hue]
  context.globalAlpha = Math.min(1, point.alpha)
  if (sprite) {
    context.drawImage(
      sprite,
      point.x - diameter / 2,
      point.y - diameter / 2,
      diameter,
      diameter
    )
    return
  }

  context.fillStyle = PARTICLE_COLORS[point.hue]
  context.fillRect(point.x, point.y, Math.max(0.7, diameter / 5), 1)
}

/** 绘制密集环体粒子，使用深度控制亮度和尺寸。 */
function drawTorusField(
  context: CanvasRenderingContext2D,
  sprites: Array<HTMLCanvasElement | null>,
  field: ParticleField,
  pointer: PointerState,
  view: ProjectionView,
  time: number
): void {
  for (let index = 0; index < field.torus.length; index += 1) {
    const particle = field.torus[index]
    const projected = field.projectedTorus[index]
    const u = particle.u + time * (0.065 + particle.drift * 0.024)
    const v =
      particle.v -
      time * (0.042 + particle.drift * 0.018) +
      Math.sin(time * 0.48 + particle.phase) * 0.018
    const ringRadius = particle.majorRadius + particle.tubeRadius * Math.cos(v)
    const x = ringRadius * Math.cos(u)
    const y = ringRadius * Math.sin(u)
    const z =
      particle.tubeRadius * Math.sin(v) * 1.16 +
      Math.sin(u * 5 + particle.phase + time * 0.7) * 0.026

    projectWorldPoint(x, y, z, view, projected)
    projected.hue = particle.hue
    const centerDistance =
      ((projected.x - view.centerX) / (view.width * 0.205)) ** 2 +
      ((projected.y - view.centerY) / (view.height * 0.185)) ** 2
    const centerMask = Math.max(0, Math.min(1, (centerDistance - 0.72) / 0.5))
    const shimmer = 0.84 + Math.sin(time * 1.15 + particle.phase) * 0.16
    projected.alpha =
      particle.alpha * (0.36 + projected.depth * 0.9) * shimmer * centerMask
    projected.scale *= particle.size

    if (projected.depth > 0.47) {
      repelProjectedPoint(projected, pointer, 7 * projected.depth)
    }
  }

  context.save()
  // 加色混合与绘制次序无关，无需逐帧排序。
  context.globalCompositeOperation = 'lighter'
  for (const point of field.projectedTorus) {
    const diameter = Math.max(2.4, point.scale * 5.4)
    drawParticle(context, sprites, point, diameter)
  }
  context.restore()
}

/** 绘制沿环体表面运动的短轨道和高亮脉冲。 */
function drawOrbitTrails(
  context: CanvasRenderingContext2D,
  sprites: Array<HTMLCanvasElement | null>,
  view: ProjectionView,
  time: number
): void {
  const trailCount = view.width < 680 ? 4 : 7
  const point: ProjectedParticle = {
    alpha: 0,
    depth: 0,
    hue: 2,
    scale: 1,
    x: 0,
    y: 0,
  }

  context.save()
  context.globalCompositeOperation = 'lighter'
  context.setLineDash([2, 4.5])
  context.lineCap = 'round'

  for (let trail = 0; trail < trailCount; trail += 1) {
    let hue = 0
    if (trail % 3 === 0) {
      hue = 2
    } else if (trail % 2 === 0) {
      hue = 3
    }
    const start = trail * 0.93 + time * (0.11 + trail * 0.008)
    const length = 0.46 + (trail % 3) * 0.12
    const baseV = trail * 0.82 + Math.sin(time * 0.16 + trail) * 0.2
    context.beginPath()

    for (let step = 0; step <= 18; step += 1) {
      const progress = step / 18
      const u = start + progress * length
      const v = baseV + Math.sin(u * 2.5 + trail) * 0.12
      const majorRadius = 1.7 + (trail % 3) * 0.06
      const tubeRadius = 0.48 + (trail % 2) * 0.08
      const ringRadius = majorRadius + tubeRadius * Math.cos(v)
      projectWorldPoint(
        ringRadius * Math.cos(u),
        ringRadius * Math.sin(u),
        tubeRadius * Math.sin(v) * 1.17,
        view,
        point
      )
      if (step === 0) {
        context.moveTo(point.x, point.y)
      } else {
        context.lineTo(point.x, point.y)
      }
    }

    context.globalAlpha = 0.22 + point.depth * 0.3
    context.strokeStyle = PARTICLE_COLORS[hue]
    context.lineWidth = 0.55 + point.depth * 0.7
    context.stroke()
    point.alpha = 0.58 + point.depth * 0.36
    point.hue = hue
    drawParticle(context, sprites, point, 7 + point.depth * 5)
  }

  context.restore()
}

/** 投影并绘制底部透视波面，固定网格邻接避免二次复杂度。 */
function drawWaveField(
  context: CanvasRenderingContext2D,
  sprites: Array<HTMLCanvasElement | null>,
  field: ParticleField,
  viewport: Viewport,
  time: number
): void {
  for (let index = 0; index < field.wave.length; index += 1) {
    const particle = field.wave[index]
    const projected = field.projectedWave[index]
    const spread = viewport.width * (0.14 + particle.depth * 0.39)
    const wave =
      Math.sin(
        particle.x * 5.2 - time * 0.85 + particle.depth * 5.6 + particle.phase
      ) *
      viewport.height *
      (0.006 + particle.depth * 0.012)
    projected.x = viewport.centerX + particle.x * spread
    projected.y = viewport.height * (0.69 + particle.depth * 0.325) + wave
    projected.depth = particle.depth
    projected.alpha =
      (0.07 + particle.depth * 0.42) *
      (0.82 + Math.sin(time * 0.7 + particle.phase) * 0.18)
    projected.hue = particle.hue
    projected.scale = particle.size * (0.65 + particle.depth * 0.8)
  }

  context.save()
  context.globalCompositeOperation = 'lighter'
  context.lineWidth = 0.45
  for (let row = 2; row < field.waveRows; row += 3) {
    context.beginPath()
    for (let column = 0; column < field.waveColumns; column += 1) {
      const point = field.projectedWave[row * field.waveColumns + column]
      if (column === 0) {
        context.moveTo(point.x, point.y)
      } else {
        context.lineTo(point.x, point.y)
      }
    }
    context.globalAlpha = 0.055 + (row / field.waveRows) * 0.16
    context.strokeStyle =
      row % 2 === 0 ? PARTICLE_COLORS[0] : PARTICLE_COLORS[1]
    context.stroke()
  }

  for (const point of field.projectedWave) {
    drawParticle(context, sprites, point, 2.6 + point.scale * 3.3)
  }
  context.restore()
}

/** 绘制外围稀疏星尘，并响应指针的视差与排斥。 */
function drawStarDust(
  context: CanvasRenderingContext2D,
  sprites: Array<HTMLCanvasElement | null>,
  field: ParticleField,
  pointer: PointerState,
  viewport: Viewport,
  time: number
): void {
  const projected: ProjectedParticle = {
    alpha: 0,
    depth: 0,
    hue: 0,
    scale: 1,
    x: 0,
    y: 0,
  }

  context.save()
  context.globalCompositeOperation = 'lighter'
  for (const star of field.stars) {
    projected.x =
      (star.x + Math.sin(time * 0.09 + star.phase) * star.driftX) *
        viewport.width +
      pointer.yaw * viewport.width * (star.depth - 0.5) * 0.18
    projected.y =
      (star.y + Math.cos(time * 0.08 + star.phase) * star.driftY) *
        viewport.height +
      pointer.pitch * viewport.height * (star.depth - 0.5) * 0.18
    projected.depth = star.depth
    projected.alpha =
      star.alpha * (0.68 + Math.sin(time * 0.7 + star.phase) * 0.32)
    projected.hue = star.hue
    projected.scale = star.size * (0.7 + star.depth * 0.75)
    repelProjectedPoint(projected, pointer, 14 * (0.45 + star.depth))
    drawParticle(context, sprites, projected, 2.4 + projected.scale * 3.4)
  }
  context.restore()
}

/** 清空透明画布并按远近顺序合成完整粒子场。 */
function drawScene(
  context: CanvasRenderingContext2D,
  sprites: Array<HTMLCanvasElement | null>,
  field: ParticleField,
  pointer: PointerState,
  viewport: Viewport,
  time: number
): void {
  context.clearRect(0, 0, viewport.width, viewport.height)

  const auraRadius = Math.min(viewport.width, viewport.height) * 0.46
  const aura = context.createRadialGradient(
    viewport.centerX,
    viewport.centerY,
    0,
    viewport.centerX,
    viewport.centerY,
    auraRadius
  )
  aura.addColorStop(0, 'rgb(10 115 158 / 0.018)')
  aura.addColorStop(0.45, 'rgb(11 171 219 / 0.035)')
  aura.addColorStop(1, 'rgb(3 7 11 / 0)')
  context.fillStyle = aura
  context.fillRect(0, 0, viewport.width, viewport.height)

  drawStarDust(context, sprites, field, pointer, viewport, time)
  drawWaveField(context, sprites, field, viewport, time)
  const view = createProjectionView(viewport, pointer, time)
  drawTorusField(context, sprites, field, pointer, view, time)
  drawOrbitTrails(context, sprites, view, time)
  context.globalAlpha = 1
  context.globalCompositeOperation = 'source-over'
}

/** 绑定画布生命周期、观察器和唯一动画循环；上下文缺失时安全退化。 */
function createParticleSceneController(
  canvas: HTMLCanvasElement
): ParticleSceneController | null {
  const context = canvas.getContext('2d')
  if (!context) {
    return null
  }

  const sprites = createParticleSprites()
  const pointer: PointerState = {
    inside: false,
    pitch: 0,
    targetPitch: 0,
    targetYaw: 0,
    x: 0,
    y: 0,
    yaw: 0,
  }
  let bounds = canvas.getBoundingClientRect()
  let viewport: Viewport | null = null
  let field: ParticleField | null = null
  let animationFrame: number | null = null
  let destroyed = false
  const canObserveIntersection = typeof IntersectionObserver !== 'undefined'
  let intersecting = canObserveIntersection
    ? bounds.bottom > 0 && bounds.top < document.documentElement.clientHeight
    : true
  let manuallyPaused = false
  let previousFrameTime = 0
  let previousPaintTime = 0
  let sceneTime = 5.4
  const motionQuery = window.matchMedia?.('(prefers-reduced-motion: reduce)')
  let reducedMotion = motionQuery?.matches ?? false

  /** 绘制当前模拟时间和交互状态。 */
  const render = (): void => {
    if (!viewport || !field) {
      return
    }
    drawScene(context, sprites, field, pointer, viewport, sceneTime)
  }

  /** 判断当前是否满足持续动画条件。 */
  const shouldAnimate = (): boolean =>
    !destroyed &&
    !manuallyPaused &&
    !reducedMotion &&
    !document.hidden &&
    intersecting

  /** 取消唯一待执行帧并清除帧间时间。 */
  const stopAnimation = (): void => {
    if (animationFrame !== null) {
      cancelAnimationFrame(animationFrame)
      animationFrame = null
    }
    previousFrameTime = 0
  }

  /** 在没有待执行帧时安排下一帧。 */
  const requestNextFrame = (): void => {
    if (!shouldAnimate() || animationFrame !== null) {
      return
    }
    animationFrame = requestAnimationFrame(runFrame)
  }

  /** 按设备宽度限制帧率，并推进粒子与视角动画。 */
  const runFrame = (now: number): void => {
    animationFrame = null
    if (!shouldAnimate()) {
      return
    }

    if (previousFrameTime === 0) {
      previousFrameTime = now
      previousPaintTime = now - 100
    }
    const frameInterval = bounds.width < 680 ? 1000 / 30 : 1000 / 45
    if (now - previousPaintTime >= frameInterval) {
      const elapsed = Math.min(0.05, (now - previousFrameTime) / 1000)
      previousFrameTime = now
      previousPaintTime = now
      sceneTime += elapsed
      pointer.yaw += (pointer.targetYaw - pointer.yaw) * 0.055
      pointer.pitch += (pointer.targetPitch - pointer.pitch) * 0.055
      render()
    }
    requestNextFrame()
  }

  /** 根据可见性、动态偏好和手动状态启停动画。 */
  const syncAnimation = (): void => {
    if (shouldAnimate()) {
      requestNextFrame()
      return
    }
    stopAnimation()
    render()
  }

  /** 刷新画布相对视口边界，供指针和滚动计算使用。 */
  const refreshBounds = (): void => {
    bounds = canvas.getBoundingClientRect()
  }

  /** 同步 CSS 尺寸、有限 DPR 缓冲区与密度档位。 */
  const resizeCanvas = (): void => {
    refreshBounds()
    const width = Math.max(1, Math.round(bounds.width))
    const height = Math.max(1, Math.round(bounds.height))
    const pixelRatio = Math.min(window.devicePixelRatio || 1, 1.5)
    const bufferWidth = Math.round(width * pixelRatio)
    const bufferHeight = Math.round(height * pixelRatio)
    const sizeChanged =
      canvas.width !== bufferWidth || canvas.height !== bufferHeight

    if (sizeChanged) {
      canvas.width = bufferWidth
      canvas.height = bufferHeight
    }
    if (sizeChanged || !field) {
      field = createParticleField(width)
    }
    context.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0)
    viewport = {
      centerX: width * 0.5,
      centerY: height * 0.48,
      height,
      width,
      xScale: (width * 0.82) / 5.1,
      yScale: (height * 0.64) / 5.1,
    }
    if (!pointer.inside) {
      pointer.x = viewport.centerX
      pointer.y = viewport.centerY
    }
    render()
  }

  /** 将视口指针映射到画布内的排斥位置和目标视角。 */
  const handlePointerMove = (event: PointerEvent): void => {
    const x = event.clientX - bounds.left
    const y = event.clientY - bounds.top
    const inside = x >= 0 && y >= 0 && x <= bounds.width && y <= bounds.height
    pointer.inside = inside
    if (!inside) {
      pointer.targetYaw = 0
      pointer.targetPitch = 0
      return
    }

    pointer.x = x
    pointer.y = y
    const safeWidth = Math.max(1, bounds.width)
    const safeHeight = Math.max(1, bounds.height)
    pointer.targetYaw = ((x / safeWidth) * 2 - 1) * 0.13
    pointer.targetPitch = ((y / safeHeight) * 2 - 1) * -0.075
  }

  /** 在页面显隐变化时同步动画。 */
  const handleVisibilityChange = (): void => {
    syncAnimation()
  }

  /** 响应系统减少动态偏好的实时变化。 */
  const handleMotionChange = (event: MediaQueryListEvent): void => {
    reducedMotion = event.matches
    syncAnimation()
  }

  const resizeObserver =
    typeof ResizeObserver === 'undefined'
      ? null
      : new ResizeObserver(() => resizeCanvas())
  resizeObserver?.observe(canvas)

  const intersectionObserver = !canObserveIntersection
    ? null
    : new IntersectionObserver((entries) => {
        intersecting = entries[0]?.isIntersecting ?? false
        syncAnimation()
      })
  intersectionObserver?.observe(canvas)

  window.addEventListener('pointermove', handlePointerMove, { passive: true })
  window.addEventListener('resize', resizeCanvas, { passive: true })
  window.addEventListener('scroll', refreshBounds, { passive: true })
  document.addEventListener('visibilitychange', handleVisibilityChange)
  motionQuery?.addEventListener('change', handleMotionChange)
  resizeCanvas()

  return {
    destroy: () => {
      destroyed = true
      stopAnimation()
      resizeObserver?.disconnect()
      intersectionObserver?.disconnect()
      window.removeEventListener('pointermove', handlePointerMove)
      window.removeEventListener('resize', resizeCanvas)
      window.removeEventListener('scroll', refreshBounds)
      document.removeEventListener('visibilitychange', handleVisibilityChange)
      motionQuery?.removeEventListener('change', handleMotionChange)
    },
    setPaused: (paused: boolean) => {
      manuallyPaused = paused
      syncAnimation()
    },
  }
}

/** 渲染首页装饰性三维粒子环；暂停或减少动态时仅保留静态帧。 */
export function ParticleScene(props: { paused?: boolean }): ReactElement {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const controllerRef = useRef<ParticleSceneController | null>(null)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) {
      return
    }

    const controller = createParticleSceneController(canvas)
    controllerRef.current = controller
    return () => {
      controller?.destroy()
      if (controllerRef.current === controller) {
        controllerRef.current = null
      }
    }
  }, [])

  useEffect(() => {
    controllerRef.current?.setPaused(props.paused === true)
  }, [props.paused])

  return (
    <canvas
      ref={canvasRef}
      className='mansui-particle-canvas'
      aria-hidden='true'
    />
  )
}
