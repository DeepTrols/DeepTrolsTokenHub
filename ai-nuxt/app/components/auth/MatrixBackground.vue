<script setup lang="ts">
const canvasRef = ref<HTMLCanvasElement | null>(null)

onMounted(() => {
  const canvasElement = canvasRef.value
  if (!canvasElement) {
    return
  }

  const drawingContext = canvasElement.getContext('2d')
  if (!drawingContext) {
    return
  }

  const canvas: HTMLCanvasElement = canvasElement
  const context: CanvasRenderingContext2D = drawingContext
  const ratio = Math.min(window.devicePixelRatio || 1, 2)
  const glyphs = '0101016789TokenHubAPI'
  let columns: number[] = []
  let animationFrame = 0

  function resize() {
    const { innerWidth, innerHeight } = window
    canvas.width = innerWidth * ratio
    canvas.height = innerHeight * ratio
    canvas.style.width = `${innerWidth}px`
    canvas.style.height = `${innerHeight}px`
    context.setTransform(ratio, 0, 0, ratio, 0, 0)
    columns = Array.from({ length: Math.ceil(innerWidth / 18) }, () => Math.random() * innerHeight)
  }

  function draw() {
    context.fillStyle = 'rgba(6, 7, 11, 0.09)'
    context.fillRect(0, 0, canvas.width / ratio, canvas.height / ratio)
    context.font = '13px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace'

    columns.forEach((y, index) => {
      const text = glyphs[Math.floor(Math.random() * glyphs.length)] ?? '0'
      const x = index * 18
      context.fillStyle = Math.random() > 0.9 ? 'rgba(255, 133, 0, .78)' : 'rgba(255, 171, 64, .2)'
      context.fillText(text, x, y)
      columns[index] = y > window.innerHeight + Math.random() * 1000 ? 0 : y + 18
    })

    animationFrame = requestAnimationFrame(draw)
  }

  resize()
  window.addEventListener('resize', resize)
  animationFrame = requestAnimationFrame(draw)

  onUnmounted(() => {
    window.removeEventListener('resize', resize)
    cancelAnimationFrame(animationFrame)
  })
})
</script>

<template>
  <canvas ref="canvasRef" class="absolute inset-0 h-full w-full opacity-[0.78]" aria-hidden="true" />
</template>
