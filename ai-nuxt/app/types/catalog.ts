export interface ModelRecord {
  model_name: string
  description?: string
  tags?: string
  icon?: string
  vendor_id?: number
  vendor_name?: string
  vendor_icon?: string
  vendor_description?: string
  quota_type: number
  model_ratio: number
  model_price: number
  completion_ratio?: number
  cache_ratio?: number
  create_cache_ratio?: number
  image_ratio?: number
  audio_ratio?: number
  audio_completion_ratio?: number
  enable_groups: string[]
  supported_endpoint_types?: string[]
}

export interface VendorRecord {
  id: number
  name: string
  icon?: string
  description?: string
}

export interface PricingPayload {
  success: boolean
  data: ModelRecord[]
  vendors: VendorRecord[]
  group_ratio: Record<string, number>
  usable_group: Record<string, string>
  supported_endpoint: Record<string, { path: string; method: string }>
  auto_groups: string[]
  pricing_version?: string
}

export interface StatusPayload {
  success: boolean
  message: string
  data: {
    custom_currency_exchange_rate: number
    custom_currency_symbol: string
    logo: string
    price: number
    quota_display_type: string
    system_name: string
    usd_exchange_rate: number
  }
}

export interface EnrichedModelRecord extends ModelRecord {
  key: string
  vendor_name: string
  vendor_icon: string
}

export interface PriceSummary {
  inputPrice: string
  completionPrice: string
  suffix: string
}

export interface PriceDetailItem {
  key: string
  label: string
  value: string
  suffix: string
}

export interface ModelPricingGroup {
  key: string
  label: string
  ratio: number
  billingType: string
  priceItems: PriceDetailItem[]
}

export interface ModelEndpointItem {
  key: string
  label: string
  path: string
  method: string
}
