//go:build ignore

package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	key := os.Getenv("DT_API_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "DT_API_KEY not set")
		os.Exit(1)
	}
	body := `{"model":"qwen3.5-flash","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`

	req, _ := http.NewRequest("POST", "http://localhost:8080/v1/chat/completions", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("X-Request-ID", "test-"+time.Now().Format("150405"))

	c := &http.Client{Timeout: 30 * time.Second}
	r, err := c.Do(req)
	if err != nil {
		fmt.Println("ERR:", err)
		return
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	l := len(b)
	if l > 500 {
		l = 500
	}
	fmt.Println("Status:", r.StatusCode)
	fmt.Println(string(b[:l]))
}
