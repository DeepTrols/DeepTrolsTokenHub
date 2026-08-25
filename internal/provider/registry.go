// Package provider defines the domestic Provider adapter registry and
// templates for the enterprise gateway (blueprint Phase 3).
//
// Registry design follows TokenHub's AdapterRegistry (Apache-2.0): providers
// declare their capabilities so routing/UI can decide which endpoints and
// upstream calls are available without hardcoding per-provider branches.
package provider

// Capability declares an upstream capability a provider template supports.
type Capability string

const (
	CapChat       Capability = "chat"
	CapChatStream Capability = "chat_stream"
	CapEmbeddings Capability = "embeddings"
	CapImages     Capability = "images"
	CapAudio      Capability = "audio"
	CapTTS        Capability = "tts"
	CapVideo      Capability = "video"
	CapModels     Capability = "models"
	CapProbe      Capability = "probe"
)

// AuthScheme describes how the upstream authenticates requests.
type AuthScheme string

const (
	AuthBearer AuthScheme = "bearer"  // Authorization: Bearer <key>
	AuthAPIKey AuthScheme = "api_key" // x-api-key: <key>
	AuthCustom AuthScheme = "custom"  // provider-specific header set by adapter
)

// Template is a domestic provider template: identity, default base URL,
// declared capabilities, and auth scheme. Actual model availability is
// discovered from the upstream and stored in the model catalog.
type Template struct {
	Type         string
	Name         string
	BaseURL      string
	AuthScheme   AuthScheme
	Capabilities []Capability
}

// Templates is the domestic-only provider catalog. Foreign providers
// (OpenAI / Anthropic / Gemini / Codex) are intentionally excluded.
var Templates = []Template{
	{
		Type: "deepseek", Name: "DeepSeek 深度求索",
		BaseURL:      "https://api.deepseek.com",
		AuthScheme:   AuthBearer,
		Capabilities: []Capability{CapChat, CapChatStream, CapModels, CapProbe},
	},
	{
		Type: "qwen", Name: "Qwen 通义千问",
		BaseURL:      "https://dashscope.aliyuncs.com/compatible-mode",
		AuthScheme:   AuthBearer,
		Capabilities: []Capability{CapChat, CapChatStream, CapEmbeddings, CapImages, CapAudio, CapTTS, CapModels, CapProbe},
	},
	{
		Type: "zhipu", Name: "智谱AI ChatGLM",
		BaseURL:      "https://open.bigmodel.cn/api/paas/v4",
		AuthScheme:   AuthBearer,
		Capabilities: []Capability{CapChat, CapChatStream, CapEmbeddings, CapImages, CapTTS, CapModels, CapProbe},
	},
	{
		Type: "moonshot", Name: "Moonshot 月之暗面 (Kimi)",
		BaseURL:      "https://api.moonshot.cn",
		AuthScheme:   AuthBearer,
		Capabilities: []Capability{CapChat, CapChatStream, CapModels, CapProbe},
	},
	{
		Type: "bytedance", Name: "字节豆包",
		BaseURL:      "https://ark.cn-beijing.volces.com/api/v3",
		AuthScheme:   AuthBearer,
		Capabilities: []Capability{CapChat, CapChatStream, CapEmbeddings, CapImages, CapAudio, CapTTS, CapVideo, CapModels, CapProbe},
	},
	{
		Type: "baidu", Name: "百度文心一言",
		BaseURL:      "https://qianfan.baidubce.com/v2",
		AuthScheme:   AuthBearer,
		Capabilities: []Capability{CapChat, CapChatStream, CapEmbeddings, CapImages, CapTTS, CapModels, CapProbe},
	},
	{
		Type: "xfyun", Name: "讯飞星火",
		BaseURL:      "https://spark-api-open.xf-yun.com/v1",
		AuthScheme:   AuthBearer,
		Capabilities: []Capability{CapChat, CapChatStream, CapAudio, CapTTS, CapModels, CapProbe},
	},
	{
		Type: "tencent", Name: "腾讯混元",
		BaseURL:      "https://api.hunyuan.cloud.tencent.com/v1",
		AuthScheme:   AuthBearer,
		Capabilities: []Capability{CapChat, CapChatStream, CapEmbeddings, CapImages, CapTTS, CapModels, CapProbe},
	},
	{
		Type: "lingyi", Name: "零一万物 Yi",
		BaseURL:      "https://api.lingyiwanwu.com/v1",
		AuthScheme:   AuthBearer,
		Capabilities: []Capability{CapChat, CapChatStream, CapModels, CapProbe},
	},
	{
		Type: "siliconflow", Name: "SiliconFlow 硅基流动",
		BaseURL:      "https://api.siliconflow.cn",
		AuthScheme:   AuthBearer,
		Capabilities: []Capability{CapChat, CapChatStream, CapEmbeddings, CapImages, CapAudio, CapTTS, CapModels, CapProbe},
	},
}

var templateByType = func() map[string]Template {
	m := make(map[string]Template, len(Templates))
	for _, t := range Templates {
		m[t.Type] = t
	}
	return m
}()

// Lookup returns the template for a provider type.
func Lookup(providerType string) (Template, bool) {
	t, ok := templateByType[providerType]
	return t, ok
}

// ValidType reports whether the provider type has a template.
func ValidType(providerType string) bool {
	_, ok := templateByType[providerType]
	return ok
}

// TemplateBaseURLs returns type → default base URL for every template.
func TemplateBaseURLs() map[string]string {
	out := make(map[string]string, len(Templates))
	for _, t := range Templates {
		out[t.Type] = t.BaseURL
	}
	return out
}

// Supports reports whether the template declares the capability.
func (t Template) Supports(c Capability) bool {
	for _, have := range t.Capabilities {
		if have == c {
			return true
		}
	}
	return false
}
