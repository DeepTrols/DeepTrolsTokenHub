//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	tok := login()
	if tok == "" {
		fmt.Println("FAIL: could not login")
		return
	}

	dsKey := os.Getenv("DEEPSEEK_API_KEY")
	qwKey := os.Getenv("QWEN_API_KEY")
	if dsKey == "" || qwKey == "" {
		fmt.Fprintln(os.Stderr, "DEEPSEEK_API_KEY and QWEN_API_KEY must be set")
		os.Exit(1)
	}

	type prov struct{ name, prov, url, key string }
	list := []prov{
		{"DeepSeek", "deepseek", "https://api.deepseek.com", dsKey},
		{"Qwen", "qwen", "https://ws-m852wcwkjo52jqef.cn-beijing.maas.aliyuncs.com/compatible-mode/v1", qwKey},
	}
	for _, p := range list {
		r := addProvider(tok, p.name, p.prov, p.url, p.key)
		fmt.Println(r)
	}
}

func login() string {
	b := []byte(`{"email":"admin","password":"admin123"}`)
	r, err := http.Post("http://localhost:8080/api/console/auth/login", "application/json", bytes.NewReader(b))
	if err != nil {
		return ""
	}
	defer r.Body.Close()
	var v struct{ Token string }
	json.NewDecoder(r.Body).Decode(&v)
	return v.Token
}

func addProvider(tok, name, prov, url, key string) string {
	b, _ := json.Marshal(map[string]string{
		"name": name, "provider": prov, "base_url": url, "api_key": key,
	})
	req, _ := http.NewRequest("POST", "http://localhost:8080/api/admin/providers", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	c := &http.Client{Timeout: 30 * time.Second}
	r, err := c.Do(req)
	if err != nil {
		return name + ": err " + err.Error()
	}
	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	return name + " [" + fmt.Sprint(r.StatusCode) + "]: " + string(body)
}
