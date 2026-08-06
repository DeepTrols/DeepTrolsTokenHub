//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func main() {
	// 1. Login as admin
	loginBody := `{"email":"deeptrols@admin.com","password":"deeptrols@2026"}`
	resp, err := http.Post("http://localhost:8080/api/console/auth/login", "application/json", bytes.NewBufferString(loginBody))
	if err != nil {
		fmt.Printf("FAIL: Login error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Login: status=%d body=%s\n", resp.StatusCode, string(body))

	var loginResp struct{ Token string }
	json.Unmarshal(body, &loginResp)
	if loginResp.Token == "" {
		fmt.Println("FAIL: No token received")
		return
	}

	// 2. Create API key
	createBody := `{"name":"test-key-from-cli"}`
	req, _ := http.NewRequest("POST", "http://localhost:8080/api/console/api-keys", bytes.NewBufferString(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+loginResp.Token)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("FAIL: Create key error: %v\n", err)
		return
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	fmt.Printf("Create API Key: status=%d body=%s\n", resp2.StatusCode, string(body2))

	var keyResp struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Plaintext string `json:"plaintext"`
		KeyPrefix string `json:"key_prefix"`
	}
	json.Unmarshal(body2, &keyResp)
	if resp2.StatusCode == 201 && keyResp.Plaintext != "" {
		fmt.Printf("SUCCESS: API key created! id=%s plaintext=%s\n", keyResp.ID, keyResp.Plaintext)
	} else {
		fmt.Printf("FAIL: status=%d expected 201, plaintext empty=%v\n", resp2.StatusCode, keyResp.Plaintext == "")
	}
}
