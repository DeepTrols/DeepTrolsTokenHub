package provider

import "strings"

// domesticProviderKeywords are keyword fragments that identify Chinese / domestic
// model vendors and platforms. Classification is "contains" based so messy
// basellm vendor strings like "alibaba (china)" or "stepfun (global)" still match.
// Anything outside this whitelist is treated as foreign and excluded.
var domesticProviderKeywords = []string{
	"alibaba", "deepseek", "qwen", "zhipu", "glm", "moonshot", "kimi",
	"minimax", "stepfun", "tencent", "xiaomi", "bytedance", "volcengine",
	"doubao", "baidu", "iflytek", "xfyun", "lingyi", "01ai", "siliconflow",
	"z.ai", "hunyuan", "sparkdesk",
}

// IsDomesticProvider reports whether a provider/vendor name refers to a Chinese
// (domestic) model vendor. Fail-closed: unknown providers are treated as foreign.
func IsDomesticProvider(providerName string) bool {
	s := strings.ToLower(strings.TrimSpace(providerName))
	if s == "" {
		return false
	}
	for _, k := range domesticProviderKeywords {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}
