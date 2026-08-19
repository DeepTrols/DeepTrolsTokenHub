//go:build ignore

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func main() {
	const (
		apiExe  = `G:\workspace\demo\DeepTrolsTokenHub\bin\api.exe`
		envFile = `G:\workspace\demo\DeepTrolsTokenHub\.env`
		outLog  = `G:\workspace\demo\DeepTrolsTokenHub\api.out.log`
		errLog  = `G:\workspace\demo\DeepTrolsTokenHub\api.err.log`
	)

	// Kill whatever currently listens on 8080.
	cmd := exec.Command("cmd", "/c", "for /f \"tokens=5\" %a in ('netstat -ano ^| findstr :8080 ^| findstr LISTENING') do taskkill /F /PID %a")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()

	// Load environment from the repo .env (authoritative), then start the API
	// with stdout/stderr appended to the same logs used by the running dev setup.
	env, err := loadEnvFile(envFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load %s: %v\n", envFile, err)
		os.Exit(1)
	}

	outF, err := os.OpenFile(outLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open %s: %v\n", outLog, err)
		os.Exit(1)
	}
	defer outF.Close()
	errF, err := os.OpenFile(errLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open %s: %v\n", errLog, err)
		os.Exit(1)
	}
	defer errF.Close()

	apiCmd := exec.Command(apiExe)
	apiCmd.Env = append(os.Environ(), env...)
	apiCmd.Stdout = outF
	apiCmd.Stderr = errF
	apiCmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008}

	if err := apiCmd.Start(); err != nil {
		fmt.Printf("Failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("API started (PID: %d)\n", apiCmd.Process.Pid)
}

// loadEnvFile parses a KEY=VALUE .env file (ignoring comments and blanks) and
// returns KEY=VALUE entries suitable for exec.Cmd.Env. Values are kept as-is.
func loadEnvFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "="); i > 0 {
			out = append(out, line)
		}
	}
	return out, sc.Err()
}
