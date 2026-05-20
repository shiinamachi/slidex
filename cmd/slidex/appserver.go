package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type appServerClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan map[string]any
	nextID int
	stderr strings.Builder
}

func newAppServerClient() (*appServerClient, error) {
	cmd := exec.Command("codex", "app-server", "--listen", "stdio://")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	client := &appServerClient{cmd: cmd, stdin: stdin, lines: make(chan map[string]any, 256)}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go client.scanStdout(stdout)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			client.stderr.WriteString(scanner.Text())
			client.stderr.WriteByte('\n')
		}
	}()
	return client, nil
}

func (c *appServerClient) scanStdout(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg map[string]any
		if json.Unmarshal([]byte(line), &msg) == nil {
			c.lines <- msg
		}
	}
	close(c.lines)
}

func (c *appServerClient) close() {
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
}

func (c *appServerClient) request(method string, params map[string]any, timeout time.Duration) (map[string]any, []map[string]any, error) {
	c.nextID++
	id := c.nextID
	req := map[string]any{"id": id, "method": method, "params": params}
	raw, _ := json.Marshal(req)
	if _, err := fmt.Fprintln(c.stdin, string(raw)); err != nil {
		return nil, nil, err
	}
	deadline := time.After(timeout)
	var notifications []map[string]any
	for {
		select {
		case msg, ok := <-c.lines:
			if !ok {
				return nil, notifications, fmt.Errorf("app-server closed stdout: %s", c.stderr.String())
			}
			if got, ok := numberAsInt(msg["id"]); ok && got == id {
				if errObj, exists := msg["error"]; exists {
					return msg, notifications, fmt.Errorf("app-server %s error: %v", method, errObj)
				}
				return msg, notifications, nil
			}
			if _, hasMethod := msg["method"]; hasMethod {
				notifications = append(notifications, msg)
			}
		case <-deadline:
			return nil, notifications, fmt.Errorf("app-server %s timed out", method)
		}
	}
}

func appServerCapabilitySnapshot(deckAbs string, startThread bool) (map[string]any, error) {
	client, err := newAppServerClient()
	if err != nil {
		return nil, err
	}
	defer client.close()
	snapshot := map[string]any{
		"schemaVersion": "slidex.protocolDiagnostics.v1",
		"generatedAt":   time.Now().UTC().Format(time.RFC3339),
		"codexVersion":  installedCodexVersion(),
		"events":        []map[string]any{},
	}
	addEvents := func(events []map[string]any) {
		if len(events) == 0 {
			return
		}
		existing, _ := snapshot["events"].([]map[string]any)
		snapshot["events"] = append(existing, events...)
	}
	initResp, events, err := client.request("initialize", map[string]any{
		"clientInfo": map[string]any{"name": "slidex", "title": "slidex CLI", "version": toolVersion},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	}, 10*time.Second)
	addEvents(events)
	if err != nil {
		return snapshot, err
	}
	snapshot["initialize"] = initResp["result"]
	for _, method := range []string{"model/list", "experimentalFeature/list", "mcpServerStatus/list"} {
		resp, events, err := client.request(method, map[string]any{}, 20*time.Second)
		addEvents(events)
		if err != nil {
			return snapshot, err
		}
		snapshot[strings.ReplaceAll(method, "/", "_")] = resp["result"]
	}
	if startThread {
		resp, events, err := client.request("thread/start", map[string]any{
			"cwd":            mustAbs("."),
			"approvalPolicy": "never",
			"sandbox":        "read-only",
			"serviceName":    "slidex",
			"model":          "gpt-5.4-mini",
		}, 20*time.Second)
		addEvents(events)
		if err != nil {
			return snapshot, err
		}
		snapshot["thread_start"] = resp["result"]
		if threadID := extractThreadID(resp["result"]); threadID != "" {
			resp, events, err = client.request("experimentalFeature/list", map[string]any{"threadId": threadID}, 20*time.Second)
			addEvents(events)
			if err != nil {
				return snapshot, err
			}
			snapshot["experimentalFeature_thread_scoped"] = resp["result"]
		}
	}
	snapshot["protocolBundle"] = filepath.ToSlash(filepath.Join("internal", "codex", "protocol", "codex-cli-"+requiredCodexVersion))
	return snapshot, nil
}

func extractThreadID(v any) string {
	obj, _ := v.(map[string]any)
	thread, _ := obj["thread"].(map[string]any)
	id, _ := thread["id"].(string)
	return id
}
