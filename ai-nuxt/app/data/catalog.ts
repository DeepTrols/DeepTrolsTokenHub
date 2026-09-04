import pricingJson from './pricing.json'
import statusJson from './status.json'
import type {
  EnrichedModelRecord,
  ModelEndpointItem,
  ModelPricingGroup,
  ModelRecord,
  PriceDetailItem,
  PriceSummary,
  PricingPayload,
  StatusPayload,
  VendorRecord,
} from '~/types/catalog'

export const pricingCatalog = pricingJson as PricingPayload
export const statusCatalog = statusJson as StatusPayload

const vendorById = new Map<number, VendorRecord>(pricingCatalog.vendors.map((vendor) => [vendor.id, vendor]))

export const vendorGradients: Record<string, string[]> = {
  OpenAI: ['#2d8a68', '#3da878', '#4dc491', '#5ed4a5'],
  DeepSeek: ['#5c6cc8', '#6e7edd', '#8090ff', '#99a5ff'],
  Qwen: ['#8a5cc0', '#9b70d5', '#ac84e8', '#bda0ff'],
  Zhipu: ['#5c7cc0', '#6d8ed5', '#7ea0e8', '#95b5f5'],
  MiniMax: ['#803878', '#94508c', '#a868a0', '#bc80b8'],
  Kimi: ['#4a6890', '#5c7ca8', '#7090c0', '#85a8d5'],
  Moonshot: ['#4a6890', '#5c7ca8', '#7090c0', '#85a8d5'],
  Doubao: ['#2e6078', '#3e7490', '#5088a8', '#68a0c0'],
  Hunyuan: ['#2e7088', '#3e84a0', '#5098b5', '#68b0c8'],
  Tencent: ['#2e7088', '#3e84a0', '#5098b5', '#68b0c8'],
}

const fallbackGradients = [
  ['#7c60b0', '#8c72c0', '#9e85d0', '#b5a0e0'],
  ['#5c68b0', '#6e7ac0', '#808cd5', '#95a0e5'],
  ['#3a8070', '#509485', '#65a898', '#7dbaa8'],
  ['#b85078', '#c86588', '#d87a98', '#e890a8'],
  ['#e89050', '#f0a065', '#f5b878', '#fccc90'],
  ['#3a8890', '#509aa5', '#65acb5', '#80c0c8'],
  ['#d87860', '#e08c75', '#e8a08a', '#f0b8a5'],
  ['#6e5cb0', '#8070c0', '#9284d0', '#a89ae0'],
]

function hash(input: string) {
  let value = 0
  for (let index = 0; index < input.length; index += 1) {
    value = (value << 5) - value + input.charCodeAt(index)
    value |= 0
  }
  return Math.abs(value)
}

export function enrichModel(model: ModelRecord): EnrichedModelRecord {
  const vendor = model.vendor_id ? vendorById.get(model.vendor_id) : undefined

  return {
    ...model,
    key: model.model_name,
    vendor_name: vendor?.name ?? '',
    vendor_icon: model.icon ?? vendor?.icon ?? '',
    vendor_description: vendor?.description ?? '',
  }
}

export function getEnrichedModels() {
  return pricingCatalog.data.map(enrichModel)
}

export function getUniqueModels(limit?: number) {
  const seen = new Set<string>()
  const models = getEnrichedModels().filter((model) => {
    if (seen.has(model.model_name)) {
      return false
    }
    seen.add(model.model_name)
    return true
  })
  const sortedModels = models.sort((first, second) => first.model_name.localeCompare(second.model_name))

  return typeof limit === 'number' ? sortedModels.slice(0, limit) : sortedModels
}

export function vendorGradient(iconOrName?: string) {
  const vendorKey = (iconOrName || '').split('.')[0] ?? ''
  const colors = vendorGradients[vendorKey] ?? fallbackGradients[hash(vendorKey || 'model') % fallbackGradients.length] ?? fallbackGradients[0]
  const [first = '#7c60b0', second = '#8c72c0', third = '#9e85d0', fourth = '#b5a0e0'] = colors ?? []

  return `linear-gradient(145deg, ${first} 0%, ${second} 35%, ${third} 60%, ${fourth} 100%)`
}

export function formatCnyPrice(amountInUsd: number, precision = 2) {
  const rate = statusCatalog.data.usd_exchange_rate || 7.2
  return `¥${(amountInUsd * rate).toFixed(precision)}`
}

export function priceSummary(model: ModelRecord): PriceSummary {
  const groups = Array.isArray(model.enable_groups) && model.enable_groups.length > 0 ? model.enable_groups : ['default']
  const groupRatio = Math.min(...groups.map((group) => pricingCatalog.group_ratio[group] ?? 1))

  if (model.quota_type === 1) {
    const price = formatCnyPrice((model.model_price || 0) * groupRatio)
    return {
      inputPrice: price,
      completionPrice: price,
      suffix: ' / 次',
    }
  }

  const base = (model.model_ratio || 0) * 2 * groupRatio
  const completionRatio = model.completion_ratio ?? 1

  return {
    inputPrice: formatCnyPrice(base),
    completionPrice: formatCnyPrice(base * completionRatio),
    suffix: ' / 1M Tokens',
  }
}

function hasNumber(value: number | undefined) {
  return value !== undefined && Number.isFinite(Number(value))
}

function groupRatio(group: string) {
  return pricingCatalog.group_ratio[group] ?? 1
}

export function modelPriceDetailItems(model: ModelRecord, group = 'default'): PriceDetailItem[] {
  const ratio = groupRatio(group)

  if (model.quota_type === 1) {
    return [
      {
        key: 'fixed',
        label: '模型价格',
        value: formatCnyPrice((model.model_price || 0) * ratio),
        suffix: ' / 次',
      },
    ]
  }

  const base = (model.model_ratio || 0) * 2 * ratio
  const unitSuffix = ' / 1M Tokens'
  const entries = [
    { key: 'input', label: '输入价格', value: base },
    { key: 'completion', label: '输出价格', value: base * (model.completion_ratio ?? 1) },
    { key: 'cache', label: '缓存读取价格', value: hasNumber(model.cache_ratio) ? base * Number(model.cache_ratio) : undefined },
    {
      key: 'create-cache',
      label: '缓存创建价格',
      value: hasNumber(model.create_cache_ratio) ? base * Number(model.create_cache_ratio) : undefined,
    },
    { key: 'image', label: '图片输入价格', value: hasNumber(model.image_ratio) ? base * Number(model.image_ratio) : undefined },
    { key: 'audio-input', label: '音频输入价格', value: hasNumber(model.audio_ratio) ? base * Number(model.audio_ratio) : undefined },
    {
      key: 'audio-output',
      label: '音频补全价格',
      value:
        hasNumber(model.audio_ratio) && hasNumber(model.audio_completion_ratio)
          ? base * Number(model.audio_ratio) * Number(model.audio_completion_ratio)
          : undefined,
    },
  ]

  return entries
    .filter((entry): entry is { key: string; label: string; value: number } => hasNumber(entry.value))
    .map((entry) => ({
      key: entry.key,
      label: entry.label,
      value: formatCnyPrice(entry.value),
      suffix: unitSuffix,
    }))
}

export function modelPricingGroups(model: ModelRecord): ModelPricingGroup[] {
  const groups = Array.isArray(model.enable_groups) && model.enable_groups.length > 0 ? model.enable_groups : ['default']
  const usableGroups = Object.keys(pricingCatalog.usable_group).filter(Boolean)
  const visibleGroups = groups.filter((group) => usableGroups.length === 0 || usableGroups.includes(group))
  const normalizedGroups = visibleGroups.length > 0 ? visibleGroups : groups

  return normalizedGroups.map((group) => ({
    key: group,
    label: pricingCatalog.usable_group[group] || `${group}分组`,
    ratio: groupRatio(group),
    billingType: model.quota_type === 1 ? '按次计费' : '按量计费',
    priceItems: modelPriceDetailItems(model, group),
  }))
}

export function modelEndpointItems(model: ModelRecord): ModelEndpointItem[] {
  return (model.supported_endpoint_types || []).map((endpointType) => {
    const endpoint = pricingCatalog.supported_endpoint[endpointType]
    const path = endpoint?.path?.replaceAll('{model}', model.model_name) || ''

    return {
      key: endpointType,
      label: endpointType,
      path,
      method: endpoint?.method || 'POST',
    }
  })
}

export function modelTags(model: ModelRecord) {
  return (model.tags || '')
    .split(/[,;，；|]/)
    .map((tag) => tag.trim())
    .filter(Boolean)
}
