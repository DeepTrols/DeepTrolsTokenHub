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
	killListeners("8080")

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

// killListeners terminates every process whose local address is
// <host>:<port> in LISTENING state (parsed from netstat -ano). A failed kill
// is logged but not fatal: the API start below will surface a bind error if
// the port is still taken.
func killListeners(port string) {
	out, err := exec.Command("netstat", "-ano").Output()
	if err != nil {
		fmt.Printf("netstat failed: %v\n", err)
		return
	}
	killed := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "LISTENING") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 5 || !strings.HasSuffix(f[1], ":"+port) {
			continue
		}
		pid := f[len(f)-1]
		if killed[pid] {
			continue
		}
		killed[pid] = true
		if err := exec.Command("taskkill", "/F", "/PID", pid).Run(); err != nil {
			fmt.Printf("taskkill %s failed: %v\n", pid, err)
		} else {
			fmt.Printf("killed listener PID %s on :%s\n", pid, port)
		}
	}
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
