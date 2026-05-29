<template>
  <div ref="host" class="viz-three" />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as THREE from 'three'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'

type ScatterPoint = { x: number; y: number; z: number; color?: string; size?: number; label?: string }

type Scatter3DSpec = {
  type?: string
  points?: ScatterPoint[]
  axes?: { x?: string; y?: string; z?: string }
  background?: string
  pointSize?: number
}

const props = defineProps<{ spec: any }>()

const host = ref<HTMLElement | null>(null)

let renderer: THREE.WebGLRenderer | null = null
let scene: THREE.Scene | null = null
let camera: THREE.PerspectiveCamera | null = null
let controls: OrbitControls | null = null
let resizeObserver: ResizeObserver | null = null
let animationFrame: number | null = null

let pointsMesh: THREE.Points | null = null
let axesHelper: THREE.AxesHelper | null = null

const parseSpec = (input: any): Scatter3DSpec => {
  const obj = (input && typeof input === 'object') ? input : {}
  return {
    type: String(obj.type || 'scatter3d'),
    points: Array.isArray(obj.points) ? obj.points : [],
    axes: (obj.axes && typeof obj.axes === 'object') ? obj.axes : undefined,
    background: typeof obj.background === 'string' ? obj.background : undefined,
    pointSize: Number.isFinite(Number(obj.pointSize)) ? Number(obj.pointSize) : undefined,
  }
}

const disposePoints = () => {
  if (!scene || !pointsMesh) return
  scene.remove(pointsMesh)
  pointsMesh.geometry.dispose()
  if (Array.isArray(pointsMesh.material)) {
    pointsMesh.material.forEach((mat) => mat.dispose())
  } else {
    pointsMesh.material.dispose()
  }
  pointsMesh = null
}

const setBackground = (value?: string) => {
  if (!scene) return
  if (!value) { scene.background = null; return }
  try {
    scene.background = new THREE.Color(value)
  } catch {
    scene.background = null
  }
}

const renderPoints = (spec: Scatter3DSpec) => {
  if (!scene) return
  disposePoints()

  const points = (spec.points || []).filter((p) =>
    Number.isFinite(Number(p?.x)) && Number.isFinite(Number(p?.y)) && Number.isFinite(Number(p?.z)),
  ) as ScatterPoint[]

  if (!points.length) return

  let minX = Infinity, maxX = -Infinity
  let minY = Infinity, maxY = -Infinity
  let minZ = Infinity, maxZ = -Infinity
  for (const p of points) {
    const x = Number(p.x), y = Number(p.y), z = Number(p.z)
    minX = Math.min(minX, x); maxX = Math.max(maxX, x)
    minY = Math.min(minY, y); maxY = Math.max(maxY, y)
    minZ = Math.min(minZ, z); maxZ = Math.max(maxZ, z)
  }
  const rangeX = Math.max(maxX - minX, 1e-9)
  const rangeY = Math.max(maxY - minY, 1e-9)
  const rangeZ = Math.max(maxZ - minZ, 1e-9)

  const positions = new Float32Array(points.length * 3)
  const colors = new Float32Array(points.length * 3)
  const fallback = new THREE.Color('#4f46e5')

  for (let i = 0; i < points.length; i++) {
    const p = points[i]
    const x = ((Number(p.x) - minX) / rangeX - 0.5) * 10
    const y = ((Number(p.y) - minY) / rangeY - 0.5) * 10
    const z = ((Number(p.z) - minZ) / rangeZ - 0.5) * 10
    positions[i * 3] = x
    positions[i * 3 + 1] = y
    positions[i * 3 + 2] = z

    const c = (() => {
      if (typeof p.color !== 'string' || !p.color.trim()) return fallback
      try { return new THREE.Color(p.color.trim()) } catch { return fallback }
    })()
    colors[i * 3] = c.r
    colors[i * 3 + 1] = c.g
    colors[i * 3 + 2] = c.b
  }

  const geometry = new THREE.BufferGeometry()
  geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3))
  geometry.setAttribute('color', new THREE.BufferAttribute(colors, 3))

  const size = Number.isFinite(Number(spec.pointSize)) ? Number(spec.pointSize) : 0.08
  const material = new THREE.PointsMaterial({
    size,
    vertexColors: true,
    sizeAttenuation: true,
    transparent: true,
    opacity: 0.95,
  })

  pointsMesh = new THREE.Points(geometry, material)
  scene.add(pointsMesh)
}

const applySpec = (rawSpec: any) => {
  if (!scene) return
  const spec = parseSpec(rawSpec)
  setBackground(spec.background)
  renderPoints(spec)
}

const resize = () => {
  if (!host.value || !renderer || !camera) return
  const rect = host.value.getBoundingClientRect()
  const w = Math.max(1, Math.floor(rect.width))
  const h = Math.max(1, Math.floor(rect.height))
  camera.aspect = w / h
  camera.updateProjectionMatrix()
  renderer.setSize(w, h, false)
}

const animate = () => {
  if (!renderer || !scene || !camera) return
  animationFrame = window.requestAnimationFrame(animate)
  controls?.update()
  renderer.render(scene, camera)
}

onMounted(() => {
  if (!host.value) return

  renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true })
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2))
  host.value.appendChild(renderer.domElement)

  scene = new THREE.Scene()
  camera = new THREE.PerspectiveCamera(55, 1, 0.1, 100)
  camera.position.set(12, 10, 12)

  controls = new OrbitControls(camera, renderer.domElement)
  controls.enableDamping = true
  controls.dampingFactor = 0.08

  const light = new THREE.DirectionalLight(0xffffff, 0.9)
  light.position.set(6, 10, 4)
  scene.add(light)
  scene.add(new THREE.AmbientLight(0xffffff, 0.35))

  axesHelper = new THREE.AxesHelper(5)
  scene.add(axesHelper)

  resize()
  applySpec(props.spec)

  resizeObserver = new ResizeObserver(() => resize())
  resizeObserver.observe(host.value)
  animate()
})

watch(() => props.spec, (next) => applySpec(next), { deep: true })

onBeforeUnmount(() => {
  if (animationFrame) window.cancelAnimationFrame(animationFrame)
  animationFrame = null

  resizeObserver?.disconnect()
  resizeObserver = null

  disposePoints()
  if (scene && axesHelper) scene.remove(axesHelper)
  axesHelper = null

  controls?.dispose()
  controls = null

  if (renderer) {
    renderer.dispose()
    if (renderer.domElement?.parentNode) {
      renderer.domElement.parentNode.removeChild(renderer.domElement)
    }
  }
  renderer = null
  scene = null
  camera = null
})
</script>

<style scoped>
.viz-three {
  width: 100%;
  height: 100%;
}
</style>
