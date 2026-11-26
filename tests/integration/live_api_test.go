//go:build integration

package integration

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Start the actual cmd/api binary against the schema verified by testDatabase.
// The child is stopped before the fixture drops any of its tables.
func startTestAPI(t *testing.T, schema string, extraEnv ...string) (baseURL, logPath string) {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "supportflow-api.exe")
	if out, err := exec.Command("go", "build", "-o", binary, "../../cmd/api").CombinedOutput(); err != nil {
		t.Fatalf("build API: %s", out)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	listener.Close()
	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(), "DB_NAME="+schema, "PORT="+port, "JWT_SECRET="+rand.Text()+rand.Text())
	// Test processes can contact only providers explicitly supplied by the fixture.
	cmd.Env = append(cmd.Env, "AI_BASE_URL=", "AI_API_KEY=", "AI_MODEL=", "AI_TIMEOUT=20s")
	cmd.Env = append(cmd.Env, extraEnv...)
	logFile, err := os.CreateTemp(t.TempDir(), "api-*.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { logFile.Close() })
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	url := "http://127.0.0.1:" + port
	client := http.Client{Timeout: time.Second}
	for i := 0; i < 60; i++ {
		resp, err := client.Get(url + "/readyz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return url, logFile.Name()
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("API readiness timeout")
	return "", ""
}

func liveRequests(t *testing.T, base string) func(method, path, token string, input any, want int) map[string]any {
	t.Helper()
	client := http.Client{Timeout: 5 * time.Second}
	return func(method, path, token string, input any, want int) map[string]any {
		t.Helper()
		data, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(method, base+path, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != want {
			t.Fatalf("%s %s returned %d, want %d", method, path, resp.StatusCode, want)
		}
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			t.Fatal("invalid JSON response")
		}
		if want >= 400 && (m["request_id"] != resp.Header.Get("X-Request-ID") || m["request_id"] == "" || len(m) != 3) {
			t.Fatal("invalid error envelope")
		}
		return m
	}
}
