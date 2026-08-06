//go:build ignore

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func main() {
	// Kill old process on port 8080
	cmd := exec.Command("cmd", "/c", "for /f \"tokens=5\" %a in ('netstat -ano ^| findstr :8080 ^| findstr LISTENING') do taskkill /F /PID %a")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()

	// Start new API
	apiCmd := exec.Command(`G:\workspace\demo\deeptrols-api\bin\api.exe`)
	apiCmd.Env = append(os.Environ(),
		"DATABASE_URL=postgresql://deeptrols:deeptrols_dev@localhost:5432/deeptrols?sslmode=disable",
		"REDIS_URL=redis://localhost:6379/0",
		"LITELLM_BASE_URL=http://localhost:4000",
		"LITELLM_MASTER_KEY=sk-litellm-master-dev",
		"JWT_SECRET=change-me-in-production-jwt-secret",
		"ENCRYPTION_KEY=abcdefghijklmnopqrstuvwxyz123456",
	)
	apiCmd.Stdout = os.Stdout
	apiCmd.Stderr = os.Stderr
	apiCmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008}

	if err := apiCmd.Start(); err != nil {
		fmt.Printf("Failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("API started (PID: %d)\n", apiCmd.Process.Pid)
}
