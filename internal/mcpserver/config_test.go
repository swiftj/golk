package mcpserver

import (
	"bytes"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"testing"
)

func TestConfigFromEnv_X_CFG_01(t *testing.T) {
	tests := []struct {
		name    string
		env     []string
		keys    map[string]string
		check   func(Config) string
		wantErr string
	}{
		{name: "empty", env: nil, keys: map[string]string{}},
		{name: "default key", env: []string{"LEGENDKEEPER_API_KEY=made-up-1"}, keys: map[string]string{"default": "made-up-1"}},
		{
			name: "named keys lower-cased",
			env:  []string{"LEGENDKEEPER_API_KEY_CAMPAIGN_2=made-up-2", "LEGENDKEEPER_API_KEY_sandbox=made-up-3", "LEGENDKEEPER_API_KEY=made-up-1"},
			keys: map[string]string{"default": "made-up-1", "campaign_2": "made-up-2", "sandbox": "made-up-3"},
		},
		{
			name: "empty values ignored",
			env:  []string{"LEGENDKEEPER_API_KEY=", "LEGENDKEEPER_API_KEY_WIKI=", "LEGENDKEEPER_API_KEY_DEFAULT=", "LEGENDKEEPER_API_KEY_BAD-NAME="},
			keys: map[string]string{},
		},
		{name: "unrelated and malformed entries ignored", env: []string{"PATH=/bin", "NOEQUALS", "LEGENDKEEPER_API_KEYS=x"}, keys: map[string]string{}},
		{name: "reserved default", env: []string{"LEGENDKEEPER_API_KEY_DEFAULT=made-up-4"}, wantErr: "reserved"},
		{name: "reserved DEFAULT in any case", env: []string{"LEGENDKEEPER_API_KEY_Default=made-up-4"}, wantErr: "reserved"},
		{name: "invalid name", env: []string{"LEGENDKEEPER_API_KEY_MY-WIKI=made-up-5"}, wantErr: "LEGENDKEEPER_API_KEY_MY-WIKI"},
		{name: "invalid name with space", env: []string{"LEGENDKEEPER_API_KEY_MY WIKI=made-up-5"}, wantErr: "letters, digits and _"},
		{name: "empty name", env: []string{"LEGENDKEEPER_API_KEY_=made-up-6"}, wantErr: "LEGENDKEEPER_API_KEY_"},
		{
			name: "base url", env: []string{"LEGENDKEEPER_BASE_URL=http://127.0.0.1:9"}, keys: map[string]string{},
			check: func(c Config) string { return c.BaseURL },
		},
		{name: "read-only true", env: []string{"LK_MCP_READ_ONLY=true"}, keys: map[string]string{}, check: readOnlyOf},
		{name: "read-only TRUE", env: []string{"LK_MCP_READ_ONLY=TRUE"}, keys: map[string]string{}, check: readOnlyOf},
		{name: "read-only 1", env: []string{"LK_MCP_READ_ONLY=1"}, keys: map[string]string{}, check: readOnlyOf},
		{name: "read-only yes", env: []string{"LK_MCP_READ_ONLY=yes"}, keys: map[string]string{}, check: readOnlyOf},
		{name: "read-only false", env: []string{"LK_MCP_READ_ONLY=false"}, keys: map[string]string{}, check: readOnlyOf},
		{name: "read-only 0", env: []string{"LK_MCP_READ_ONLY=0"}, keys: map[string]string{}, check: readOnlyOf},
		{name: "read-only empty", env: []string{"LK_MCP_READ_ONLY="}, keys: map[string]string{}, check: readOnlyOf},
		{name: "read-only invalid", env: []string{"LK_MCP_READ_ONLY=maybe"}, wantErr: "LK_MCP_READ_ONLY must be true or false"},
		{name: "guard off", env: []string{"LK_MCP_GUARD=off"}, keys: map[string]string{}, check: guardOf},
		{name: "guard OFF", env: []string{"LK_MCP_GUARD=OFF"}, keys: map[string]string{}, check: guardOf},
		{name: "guard on", env: []string{"LK_MCP_GUARD=on"}, keys: map[string]string{}, check: guardOf},
		{name: "guard empty", env: []string{"LK_MCP_GUARD="}, keys: map[string]string{}, check: guardOf},
		{name: "guard false", env: []string{"LK_MCP_GUARD=false"}, keys: map[string]string{}, check: guardOf},
		{name: "guard true", env: []string{"LK_MCP_GUARD=true"}, keys: map[string]string{}, check: guardOf},
		{name: "guard invalid", env: []string{"LK_MCP_GUARD=maybe"}, wantErr: "LK_MCP_GUARD must be on or off"},
		{name: "guard file", env: []string{"LK_MCP_GUARD_FILE=/elsewhere/terms.txt"}, keys: map[string]string{}, check: guardOf},
		{name: "log level debug", env: []string{"LK_MCP_LOG_LEVEL=debug"}, keys: map[string]string{}, check: levelOf},
		{name: "log level WARN", env: []string{"LK_MCP_LOG_LEVEL=WARN"}, keys: map[string]string{}, check: levelOf},
		{name: "log level error", env: []string{"LK_MCP_LOG_LEVEL=error"}, keys: map[string]string{}, check: levelOf},
		{name: "log level default", env: []string{"LK_MCP_LOG_LEVEL="}, keys: map[string]string{}, check: levelOf},
		{name: "log level invalid", env: []string{"LK_MCP_LOG_LEVEL=loud"}, wantErr: "LK_MCP_LOG_LEVEL"},
	}
	wantCheck := map[string]string{
		"base url":       "http://127.0.0.1:9",
		"read-only true": "true", "read-only TRUE": "true", "read-only 1": "true", "read-only yes": "true",
		"read-only false": "false", "read-only 0": "false", "read-only empty": "false",
		"guard off": "off=true file=", "guard OFF": "off=true file=", "guard on": "off=false file=",
		"guard empty": "off=false file=", "guard file": "off=false file=/elsewhere/terms.txt",
		"guard false": "off=true file=", "guard true": "off=false file=",
		"log level debug": "DEBUG", "log level WARN": "WARN", "log level error": "ERROR", "log level default": "INFO",
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := ConfigFromEnv(tt.env)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				for _, kv := range tt.env {
					if _, v, _ := strings.Cut(kv, "="); v != "" && strings.HasPrefix(v, "made-up") && strings.Contains(err.Error(), v) {
						t.Errorf("error holds the key value: %v", err)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(c.keys, tt.keys) {
				t.Errorf("keys = %v, want %v", c.keys, tt.keys)
			}
			if tt.check != nil {
				if got, want := tt.check(c), wantCheck[tt.name]; got != want {
					t.Errorf("got %q, want %q", got, want)
				}
			}
			if tt.check == nil || !strings.HasPrefix(tt.name, "log level") {
				if c.LogLevel != slog.LevelInfo {
					t.Errorf("log level = %v, want INFO", c.LogLevel)
				}
			}
		})
	}
}

func readOnlyOf(c Config) string { return fmt.Sprint(c.ReadOnly) }
func guardOf(c Config) string    { return fmt.Sprintf("off=%t file=%s", c.GuardOff, c.GuardFile) }
func levelOf(c Config) string    { return c.LogLevel.String() }

func TestConfig_X_CFG_02_NoKeyInFormatting(t *testing.T) {
	const key1, key2 = "made-up-key-0f3a9c", "made-up-key-77be21"
	c, err := ConfigFromEnv([]string{"LEGENDKEEPER_API_KEY=" + key1, "LEGENDKEEPER_API_KEY_SECOND=" + key2, "LK_MCP_GUARD=off"})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("starting", "config", c, "ptr", &c)
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("starting", "config", c, "ptr", &c)
	outputs := map[string]string{
		"String":       c.String(),
		"Sprintf":      fmt.Sprintf("%v %+v %#v %s", c, c, c, c),
		"Sprintf ptr":  fmt.Sprintf("%v %+v %#v %s", &c, &c, &c, &c),
		"Sprint slice": fmt.Sprint([]Config{c}),
		"slog":         buf.String(),
	}
	for name, s := range outputs {
		if strings.Contains(s, key1) || strings.Contains(s, key2) {
			t.Errorf("%s holds a key: %s", name, s)
		}
	}
	if s := c.String(); !strings.Contains(s, "default") || !strings.Contains(s, "second") || !strings.Contains(s, "guard off") {
		t.Errorf("String() = %q; it should name the projects and guard state", s)
	}
}

func TestConfig_X_CFG_01_WithKeyAndProjects(t *testing.T) {
	c := Config{}.WithKey("b", "made-up-b").WithKey("a", "made-up-a")
	d := c.WithKey("a", "made-up-a2")
	if got := strings.Join(c.Projects(), ","); got != "a,b" {
		t.Errorf("Projects() = %s", got)
	}
	if c.keys["a"] != "made-up-a" || d.keys["a"] != "made-up-a2" {
		t.Error("WithKey changed the original config")
	}
}
