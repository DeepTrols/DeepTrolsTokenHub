<script setup lang="ts">
interface Star {
  x: number
  y: number
  r: number
  twinkleSpeed: number
  twinklePhase: number
  baseAlpha: number
  twinkleAmp: number
}

interface OrbitParticle {
  x: number
  y: number
  cx: number
  cy: number
  r: number
  angle: number
  aspectY: number
  orbitalSpeed: number
  particleR: number
  alpha: number
  hue: number
  trail: Array<{ x: number; y: number }>
  maxTrail: number
  life: number
  decay: number
  spiralRate: number
  lineTrail: boolean
}

const canvasRef = ref<HTMLCanvasElement | null>(null)

onMounted(() => {
  const canvasElement = canvasRef.value

  if (!canvasElement) {
    return
  }

  const parentElement = canvasElement.parentElement
  const drawingContext = canvasElement.getContext('2d')
  if (!parentElement || !drawingContext) {
    return
  }

  const canvas: HTMLCanvasElement = canvasElement
  const parent: HTMLElement = parentElement
  const context: CanvasRenderingContext2D = drawingContext
  const ratio = Math.min(window.devicePixelRatio || 1, 2)
  const stars: Star[] = []
  const particles: OrbitParticle[] = []
  let width = 0
  let height = 0
  let time = 0
  let lastFrame = 0
  let animationFrame = 0
  let backgroundGradient: CanvasGradient | null = null
  let vignetteGradient: CanvasGradient | null = null
  const starCount = 120
  const maxParticles = 180
  const ellipticTilt = 0.35

  function createStars() {
    stars.length = 0
    for (let index = 0; index < starCount; index += 1) {
      const kind = Math.random()
      let radius: number
      let twinkleAmp: number
      let twinkleSpeed: number
      let baseAlpha: number

      if (kind < 0.3) {
        radius = Math.random() * 2 + 0.8
        twinkleAmp = 0.4 + Math.random() * 0.5
        twinkleSpeed = 0.3 + Math.random() * 2
        baseAlpha = 0.5 + Math.random() * 0.5
      } else if (kind < 0.7) {
        radius = Math.random() * 1 + 0.3
        twinkleAmp = 0.15 + Math.random() * 0.25
        twinkleSpeed = 0.15 + Math.random()
        baseAlpha = 0.3 + Math.random() * 0.5
      } else {
        radius = Math.random() * 0.5 + 0.12
        twinkleAmp = 0.05 + Math.random() * 0.12
        twinkleSpeed = 0.1 + Math.random() * 0.6
        baseAlpha = 0.15 + Math.random() * 0.3
      }

      stars.push({
        x: Math.random() * width,
        y: Math.random() * height,
        r: radius,
        twinkleSpeed,
        twinklePhase: Math.random() * Math.PI * 2,
        baseAlpha,
        twinkleAmp,
      })
    }
  }

  function createParticle() {
    const centerX = width * 0.5
    const centerY = height * 0.5
    const radiusLimit = Math.max(width, height) * 0.5
    const angle = Math.random() * Math.PI * 2
    const radius = radiusLimit * Math.sqrt(Math.random())
    const aspectY = 0.6 + Math.random() * 0.4
    const x = centerX + Math.cos(angle) * radius
    const y = centerY + Math.sin(angle) * radius * aspectY + ellipticTilt * Math.cos(angle) * radius
    const isLine = Math.random() < 0.1

    particles.push({
      x,
      y,
      cx: centerX,
      cy: centerY,
      r: radius,
      angle,
      aspectY,
      orbitalSpeed: 0.08 + Math.random() * 0.4,
      particleR: isLine ? 0.5 + Math.random() * 1.2 : 0.8 + Math.random() * 2.2,
      alpha: 0.5 + Math.random() * 0.48,
      hue: 18 + Math.random() * 12,
      trail: [],
      maxTrail: isLine ? 8 + Math.floor(Math.random() * 10) : 18 + Math.floor(Math.random() * 18),
      life: 1,
      decay: 0.003 + Math.random() * 0.01,
      spiralRate: 0.15 + Math.random() * 0.6,
      lineTrail: isLine,
    })
  }

  function resize() {
    const rect = parent.getBoundingClientRect()
    width = rect.width
    height = rect.height
    canvas.width = width * ratio
    canvas.height = height * ratio
    canvas.style.width = `${width}px`
    canvas.style.height = `${height}px`
    context.setTransform(ratio, 0, 0, ratio, 0, 0)
    backgroundGradient = null
    vignetteGradient = null
    particles.length = 0
    createStars()
  }

  function draw(frame: number) {
    if (!lastFrame) {
      lastFrame = frame
    }

    const delta = Math.min((frame - lastFrame) / 1000, 0.1)
    lastFrame = frame
    time += delta

    context.clearRect(0, 0, width, height)

    if (!backgroundGradient) {
      backgroundGradient = context.createRadialGradient(width * 0.92, height * 0.08, 0, width * 0.5, height * 0.5, Math.max(width, height) * 1.15)
      backgroundGradient.addColorStop(0, '#111128')
      backgroundGradient.addColorStop(0.25, '#0a0a1c')
      backgroundGradient.addColorStop(0.55, '#040410')
      backgroundGradient.addColorStop(1, '#020208')
    }

    context.fillStyle = backgroundGradient
    context.fillRect(0, 0, width, height)

    while (particles.length < maxParticles) {
      createParticle()
    }

    const minimumRadius = Math.min(width, height) * 0.02
    for (let index = particles.length - 1; index >= 0; index -= 1) {
      const particle = particles[index]
      if (!particle) {
        continue
      }

      particle.trail.push({ x: particle.x, y: particle.y })
      if (particle.trail.length > particle.maxTrail) {
        particle.trail.shift()
      }

      particle.angle += particle.orbitalSpeed * delta
      particle.r -= particle.spiralRate * delta
      particle.x = particle.cx + Math.cos(particle.angle) * particle.r
      particle.y = particle.cy + Math.sin(particle.angle) * particle.r * particle.aspectY + ellipticTilt * Math.cos(particle.angle) * particle.r
      particle.life -= particle.decay * delta

      if (particle.r < minimumRadius || particle.life <= 0) {
        particles.splice(index, 1)
        continue
      }

      if (particle.lineTrail) {
        if (particle.trail.length > 1) {
          const startPoint = particle.trail[0]
          if (!startPoint) {
            continue
          }

          context.beginPath()
          context.moveTo(startPoint.x, startPoint.y)
          for (let trailIndex = 1; trailIndex < particle.trail.length; trailIndex += 1) {
            const trailPoint = particle.trail[trailIndex]
            if (!trailPoint) {
              continue
            }
            context.lineTo(trailPoint.x, trailPoint.y)
          }
          context.strokeStyle = `hsla(${particle.hue}, 90%, 55%, ${particle.alpha * 0.7})`
          context.lineWidth = particle.particleR * 1.5
          context.lineCap = 'round'
          context.lineJoin = 'round'
          context.stroke()
          context.strokeStyle = `hsla(${particle.hue}, 70%, 60%, ${particle.alpha * 0.3})`
          context.lineWidth = particle.particleR * 3
          context.stroke()
        }
      } else {
        particle.trail.forEach((point, trailIndex) => {
          const weight = (trailIndex + 1) / particle.trail.length
          const alpha = particle.alpha * weight * 0.7
          const radius = particle.particleR * weight * 0.75
          if (alpha < 0.003) {
            return
          }
          context.beginPath()
          context.arc(point.x, point.y, radius, 0, Math.PI * 2)
          context.fillStyle = `hsla(${particle.hue}, 90%, 55%, ${alpha})`
          context.fill()
        })
      }

      context.beginPath()
      context.arc(particle.x, particle.y, particle.particleR, 0, Math.PI * 2)
      context.fillStyle = `hsla(${particle.hue}, 90%, 55%, ${particle.alpha * 0.95})`
      context.fill()
      context.beginPath()
      context.arc(particle.x, particle.y, particle.particleR * 3.5, 0, Math.PI * 2)
      context.fillStyle = `hsla(${particle.hue}, 85%, 50%, ${particle.alpha * 0.12})`
      context.fill()
    }

    stars.forEach((star) => {
      const twinkle = Math.sin(time * star.twinkleSpeed + star.twinklePhase)
      const alpha = Math.max(0.02, Math.min(1, star.baseAlpha + twinkle * star.twinkleAmp))
      context.beginPath()
      context.arc(star.x, star.y, star.r, 0, Math.PI * 2)
      context.fillStyle = `rgba(255, 255, 255, ${alpha})`
      context.fill()

      if (star.twinkleAmp > 0.35 && star.r > 0.6 && twinkle > 0.3) {
        context.beginPath()
        context.arc(star.x, star.y, star.r * 5, 0, Math.PI * 2)
        context.fillStyle = `rgba(255, 240, 220, ${alpha * twinkle * 0.12})`
        context.fill()
      }
    })

    if (!vignetteGradient) {
      vignetteGradient = context.createRadialGradient(width * 0.92, height * 0.08, Math.min(width, height) * 0.15, width * 0.5, height * 0.5, Math.max(width, height) * 0.9)
      vignetteGradient.addColorStop(0, 'rgba(0,0,0,0)')
      vignetteGradient.addColorStop(1, 'rgba(0,0,0,0.55)')
    }

    context.fillStyle = vignetteGradient
    context.fillRect(0, 0, width, height)
    animationFrame = requestAnimationFrame(draw)
  }

  resize()
  const resizeObserver = new ResizeObserver(resize)
  resizeObserver.observe(parent)
  animationFrame = requestAnimationFrame(draw)

  onUnmounted(() => {
    cancelAnimationFrame(animationFrame)
    resizeObserver.disconnect()
  })
})
</script>

<template>
  <canvas ref="canvasRef" class="absolute inset-0 h-full w-full" aria-hidden="true" />
</template>
