package relayconvert

import "testing"

func TestOpenAIToGeminiRequest(t *testing.T) {
	req := &OpenAIChatRequest{
		Model: "gemini-2.5-flash",
		Messages: []OpenAIMessage{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hi"},
		},
	}
	g := OpenAIToGeminiRequest(req)
	if g.SystemInstruction == nil || g.SystemInstruction.Parts[0].Text != "sys" {
		t.Fatalf("systemInstruction: %+v", g.SystemInstruction)
	}
	if len(g.Contents) != 1 || g.Contents[0].Role != "user" || g.Contents[0].Parts[0].Text != "hi" {
		t.Fatalf("contents: %+v", g.Contents)
	}
}

func TestGeminiToOpenAIChatResponse(t *testing.T) {
	resp := &GeminiResponse{
		Candidates:    []GeminiCandidate{{Content: GeminiContent{Role: "model", Parts: []GeminiPart{{Text: "hey"}}}, FinishReason: "MAX_TOKENS"}},
		UsageMetadata: GeminiUsage{PromptTokenCount: 8, CandidatesTokenCount: 3, TotalTokenCount: 11},
	}
	oai := GeminiToOpenAIChatResponse(resp, "gemini-2.5-flash")
	if oai.Choices[0].Message.Content != "hey" || oai.Choices[0].FinishReason != "length" {
		t.Fatalf("resp: %+v", oai.Choices[0])
	}
	if oai.Usage.TotalTokens != 11 {
		t.Fatalf("usage: %+v", oai.Usage)
	}
}
