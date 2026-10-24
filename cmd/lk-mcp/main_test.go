package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// rtKey is a made-up API key that must appear in neither output stream.
const rtKey = "made-up-key-5e0c7a21"

// buildBinary builds lk-mcp into a temporary directory, without cgo. It
// keeps the caller's environment, so GOCACHE and friends are honoured.
func buildBinary(t *testing.T) string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not on PATH")
	}
	bin := filepath.Join(t.TempDir(), "lk-mcp")
	cmd := exec.CommandContext(t.Context(), goBin, "build", "-o", bin, ".")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func TestMain_X_LOG_01_StdoutCarriesOnlyProtocol(t *testing.T) {
	bin := buildBinary(t)
	var hits sync.Map
	lkFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Store(r.Method+" "+r.URL.Path, true)
		http.Error(w, "not scripted", http.StatusTeapot)
	}))
	defer lkFake.Close()

	cmd := exec.CommandContext(t.Context(), bin)
	cmd.Env = []string{
		"LEGENDKEEPER_API_KEY=" + rtKey,
		"LEGENDKEEPER_BASE_URL=" + lkFake.URL,
		"LK_MCP_LOG_LEVEL=debug",
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	// Read stdout; signal when the tools/list response (id 2) arrives.
	var lines []string
	gotList := make(chan struct{})
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		sc := bufio.NewScanner(stdoutPipe)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		once := sync.Once{}
		for sc.Scan() {
			l := sc.Text()
			lines = append(lines, l)
			if strings.Contains(l, `"id":2`) {
				once.Do(func() { close(gotList) })
			}
		}
	}()

	for _, msg := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	} {
		if _, err := io.WriteString(stdin, msg+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-gotList:
	case <-time.After(30 * time.Second):
		t.Fatalf("no tools/list response; stderr:\n%s", stderr.String())
	}
	_ = stdin.Close()
	select {
	case <-readDone:
	case <-time.After(30 * time.Second):
		t.Fatal("stdout not closed after stdin closed")
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()
	select {
	case err := <-waitErr:
		if err != nil {
			t.Logf("exit after stdin closed: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("lk-mcp did not exit after stdin closed")
	}

	if len(lines) < 2 {
		t.Fatalf("stdout lines: %q", lines)
	}
	var sawInit, sawTools bool
	for i, l := range lines {
		var frame struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Result  struct {
				ServerInfo struct{ Name string }   `json:"serverInfo"`
				Tools      []struct{ Name string } `json:"tools"`
			} `json:"result"`
		}
		dec := json.NewDecoder(strings.NewReader(l))
		if err := dec.Decode(&frame); err != nil || dec.More() || frame.JSONRPC != "2.0" {
			t.Errorf("stdout line %d is not one JSON-RPC 2.0 object (%v): %s", i, err, l)
			continue
		}
		switch string(frame.ID) {
		case "1":
			sawInit = frame.Result.ServerInfo.Name == "lk-mcp"
		case "2":
			for _, tool := range frame.Result.Tools {
				if tool.Name == "search" {
					sawTools = true
				}
			}
		}
	}
	if !sawInit || !sawTools {
		t.Errorf("initialize ok %v, tools/list with search %v; stdout:\n%s", sawInit, sawTools, strings.Join(lines, "\n"))
	}
	errText := stderr.String()
	if !strings.Contains(errText, "level=INFO") || !strings.Contains(errText, "msg=starting") {
		t.Errorf("stderr lacks the slog start line:\n%s", errText)
	}
	for name, s := range map[string]string{"stdout": strings.Join(lines, "\n"), "stderr": errText} {
		if strings.Contains(s, rtKey) {
			t.Errorf("%s holds the API key", name)
		}
	}
	hits.Range(func(k, _ any) bool {
		t.Errorf("unexpected LegendKeeper request %v", k)
		return true
	})
}

func TestMain_X_LOG_01_Version(t *testing.T) {
	bin := buildBinary(t)
	for _, arg := range []string{"--version", "-version"} {
		cmd := exec.CommandContext(t.Context(), bin, arg)
		cmd.Env = []string{"LEGENDKEEPER_API_KEY=" + rtKey}
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\n%s", arg, err, stderr.String())
		}
		if got := stdout.String(); got != "lk-mcp dev\n" {
			t.Errorf("%s stdout = %q", arg, got)
		}
		if stderr.Len() != 0 {
			t.Errorf("%s stderr = %q", arg, stderr.String())
		}
	}
}
