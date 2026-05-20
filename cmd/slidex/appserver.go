package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
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
	stage  string
}

type appServerWorkflowRun struct {
	client   *appServerClient
	deckAbs  string
	outDir   string
	threadID string
	snapshot map[string]any
}

type appServerTurnResult struct {
	SchemaVersion    string           `json:"schemaVersion"`
	GeneratedAt      string           `json:"generatedAt"`
	Stage            string           `json:"stage"`
	ThreadID         string           `json:"threadId"`
	TurnID           string           `json:"turnId"`
	OutputSchemaPath string           `json:"outputSchemaPath"`
	OutputSchemaHash string           `json:"outputSchemaHash"`
	PromptSha256     string           `json:"promptSha256"`
	StartResponse    any              `json:"startResponse,omitempty"`
	Completion       any              `json:"completion,omitempty"`
	ThreadRead       any              `json:"threadRead,omitempty"`
	ThreadReadError  string           `json:"threadReadError,omitempty"`
	TurnsList        any              `json:"turnsList,omitempty"`
	TurnsListError   string           `json:"turnsListError,omitempty"`
	FinalMessage     string           `json:"finalMessage,omitempty"`
	StructuredOutput map[string]any   `json:"structuredOutput,omitempty"`
	AuditCorrection  any              `json:"auditCorrection,omitempty"`
	Events           []map[string]any `json:"events"`
	EventLog         string           `json:"eventLog,omitempty"`
}

func newAppServerClient() (*appServerClient, error) {
	return newAppServerClientCommand("codex", "app-server", "--listen", "stdio://")
}

func newAppServerProxyClient(sock string) (*appServerClient, error) {
	return newAppServerClientCommand("codex", "app-server", "proxy", "--sock", sock)
}

func newUnixAppServerClient(sock string) (*appServerClient, error) {
	conn, err := net.DialTimeout("unix", sock, 10*time.Second)
	if err != nil {
		return nil, err
	}
	client := &appServerClient{stdin: conn, lines: make(chan map[string]any, 256)}
	go client.scanStdout(conn)
	return client, nil
}

func newAppServerClientCommand(name string, args ...string) (*appServerClient, error) {
	cmd := exec.Command(name, args...)
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
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	if c.cmd != nil {
		_ = c.cmd.Wait()
	}
}

func (c *appServerClient) notify(method string, params map[string]any) error {
	req := map[string]any{"method": method}
	if params != nil {
		req["params"] = params
	}
	raw, _ := json.Marshal(req)
	_, err := fmt.Fprintln(c.stdin, string(raw))
	return err
}

func (c *appServerClient) request(method string, params map[string]any, timeout time.Duration) (map[string]any, []map[string]any, error) {
	if isDangerousAppServerMethod(method) {
		allowed, err := dangerousAppServerMethodAllowed(method, c.stage)
		if err != nil {
			return nil, nil, err
		}
		if !allowed {
			return nil, nil, exitCodeError(4, "dangerous App Server method %s is disabled for stage %q; add an exact stage allowlist entry to slidex.toml", method, firstNonEmpty(c.stage, "*"))
		}
	}
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

func (c *appServerClient) waitForTurnCompletion(threadID, turnID string, timeout time.Duration) ([]map[string]any, map[string]any, error) {
	deadline := time.After(timeout)
	var notifications []map[string]any
	activeTurnID := turnID
	for {
		select {
		case msg, ok := <-c.lines:
			if !ok {
				return notifications, nil, fmt.Errorf("app-server closed stdout while waiting for turn completion: %s", c.stderr.String())
			}
			if _, hasMethod := msg["method"]; hasMethod {
				notifications = append(notifications, msg)
			}
			method, _ := msg["method"].(string)
			params, _ := msg["params"].(map[string]any)
			if method == "turn/started" || method == "item/started" || method == "item/completed" {
				if paramsThreadID, _ := params["threadId"].(string); paramsThreadID == "" || paramsThreadID == threadID {
					if got := turnIDFromNotification(method, params); got != "" {
						activeTurnID = got
					}
				}
			}
			if method == "turn/completed" {
				params, _ := msg["params"].(map[string]any)
				if paramsThreadID, _ := params["threadId"].(string); paramsThreadID != "" && paramsThreadID != threadID {
					continue
				}
				turn, _ := params["turn"].(map[string]any)
				if id, _ := turn["id"].(string); id != "" && id == activeTurnID {
					return notifications, params, nil
				}
			}
		case <-deadline:
			return notifications, nil, fmt.Errorf("app-server turn/start timed out waiting for turn %s", turnID)
		}
	}
}

func (c *appServerClient) waitForThreadCompacted(threadID string, timeout time.Duration) ([]map[string]any, map[string]any, error) {
	deadline := time.After(timeout)
	var notifications []map[string]any
	for {
		select {
		case msg, ok := <-c.lines:
			if !ok {
				return notifications, nil, fmt.Errorf("app-server closed stdout while waiting for thread compact: %s", c.stderr.String())
			}
			if _, hasMethod := msg["method"]; hasMethod {
				notifications = append(notifications, msg)
			}
			method, _ := msg["method"].(string)
			if method != "thread/compacted" {
				continue
			}
			params, _ := msg["params"].(map[string]any)
			if paramsThreadID, _ := params["threadId"].(string); paramsThreadID != "" && paramsThreadID != threadID {
				continue
			}
			return notifications, params, nil
		case <-deadline:
			return notifications, nil, fmt.Errorf("app-server thread/compact/start timed out waiting for thread %s", threadID)
		}
	}
}

func startAppServerWorkflowRun(deckAbs string) (*appServerWorkflowRun, error) {
	client, err := newAppServerClient()
	if err != nil {
		return nil, err
	}
	run := &appServerWorkflowRun{
		client:  client,
		deckAbs: deckAbs,
		outDir:  filepath.Join(deckAbs, "out"),
		snapshot: map[string]any{
			"schemaVersion": "slidex.protocolDiagnostics.v1",
			"generatedAt":   time.Now().UTC().Format(time.RFC3339),
			"codexVersion":  installedCodexVersion(),
			"events":        []map[string]any{},
		},
	}
	addEvents := func(events []map[string]any) {
		if len(events) == 0 {
			return
		}
		existing, _ := run.snapshot["events"].([]map[string]any)
		run.snapshot["events"] = append(existing, events...)
	}
	initResp, events, err := client.request("initialize", map[string]any{
		"clientInfo": map[string]any{"name": "slidex", "title": "slidex CLI", "version": toolVersion},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	}, 10*time.Second)
	addEvents(events)
	if err != nil {
		client.close()
		return nil, err
	}
	run.snapshot["initialize"] = initResp["result"]
	if err := client.notify("initialized", nil); err != nil {
		client.close()
		return nil, err
	}
	run.snapshot["initialized"] = true
	for _, method := range []string{"model/list", "experimentalFeature/list", "mcpServerStatus/list"} {
		resp, events, err := client.request(method, map[string]any{}, 20*time.Second)
		addEvents(events)
		if err != nil {
			client.close()
			return nil, err
		}
		run.snapshot[strings.ReplaceAll(method, "/", "_")] = resp["result"]
	}
	resp, events, err := client.request("thread/start", map[string]any{
		"cwd":                   mustAbs("."),
		"approvalPolicy":        "never",
		"sandbox":               "read-only",
		"serviceName":           "slidex",
		"model":                 defaultCodexModel(),
		"runtimeWorkspaceRoots": uniqueStrings([]string{mustAbs("."), deckAbs}),
	}, 20*time.Second)
	addEvents(events)
	if err != nil {
		client.close()
		return nil, err
	}
	run.snapshot["thread_start"] = resp["result"]
	run.threadID = extractThreadID(resp["result"])
	if run.threadID != "" {
		resp, events, err = client.request("experimentalFeature/list", map[string]any{"threadId": run.threadID}, 20*time.Second)
		addEvents(events)
		if err != nil {
			client.close()
			return nil, err
		}
		run.snapshot["experimentalFeature_thread_scoped"] = resp["result"]
	}
	run.snapshot["protocolBundle"] = filepath.ToSlash(filepath.Join("internal", "codex", "protocol", "codex-cli-"+requiredCodexVersion))
	return run, nil
}

func (r *appServerWorkflowRun) close() {
	if r != nil && r.client != nil {
		r.client.close()
	}
}

func (r *appServerWorkflowRun) runStructuredTurn(stage, prompt, schemaPath string, timeout time.Duration) (appServerTurnResult, error) {
	return r.runStructuredTurnWithInput(stage, []map[string]any{{"type": "text", "text": prompt}}, prompt, schemaPath, timeout)
}

func (r *appServerWorkflowRun) runStructuredTurnWithInput(stage string, input []map[string]any, promptForHash, schemaPath string, timeout time.Duration) (appServerTurnResult, error) {
	previousStage := r.client.stage
	r.client.stage = stage
	defer func() { r.client.stage = previousStage }()

	result := appServerTurnResult{
		SchemaVersion:    "slidex.appServerTurn.v1",
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		Stage:            stage,
		ThreadID:         r.threadID,
		OutputSchemaPath: filepath.ToSlash(schemaPath),
		OutputSchemaHash: mustSHA256(schemaPath),
		PromptSha256:     sha256Bytes([]byte(promptForHash)),
		Events:           []map[string]any{},
	}
	schema, err := readJSONSchemaObject(schemaPath)
	if err != nil {
		return result, err
	}
	resp, events, err := r.client.request("turn/start", map[string]any{
		"threadId":              r.threadID,
		"cwd":                   mustAbs("."),
		"approvalPolicy":        "never",
		"sandboxPolicy":         map[string]any{"type": "readOnly"},
		"model":                 defaultCodexModel(),
		"runtimeWorkspaceRoots": uniqueStrings([]string{mustAbs("."), r.deckAbs}),
		"input":                 input,
		"outputSchema":          schema,
	}, 30*time.Second)
	result.Events = append(result.Events, events...)
	if err != nil {
		return result, err
	}
	result.StartResponse = resp["result"]
	result.TurnID = extractTurnID(resp["result"])
	if result.TurnID == "" {
		return result, fmt.Errorf("app-server turn/start did not return a turn id")
	}
	events, completion, err := r.client.waitForTurnCompletion(r.threadID, result.TurnID, timeout)
	result.Events = append(result.Events, events...)
	result.Completion = completion
	if err != nil {
		return result, err
	}
	if actualTurnID := turnIDFromCompletion(completion); actualTurnID != "" {
		result.TurnID = actualTurnID
	}
	if turnStatus(completion) != "completed" {
		return result, fmt.Errorf("app-server turn %s did not complete successfully: status=%s error=%v", result.TurnID, turnStatus(completion), turnError(completion))
	}
	readResp, readEvents, readErr := r.client.request("thread/read", map[string]any{"threadId": r.threadID, "includeTurns": true}, 20*time.Second)
	result.Events = append(result.Events, readEvents...)
	if readErr != nil {
		result.ThreadReadError = readErr.Error()
	} else {
		result.ThreadRead = readResp["result"]
	}
	turnsResp, turnsEvents, turnsErr := r.client.request("thread/turns/list", map[string]any{"threadId": r.threadID, "itemsView": "full", "limit": 20, "sortDirection": "desc"}, 20*time.Second)
	result.Events = append(result.Events, turnsEvents...)
	if turnsErr != nil {
		result.TurnsListError = turnsErr.Error()
	} else {
		result.TurnsList = turnsResp["result"]
	}
	finalMessage := extractFinalAgentTextFromEvents(result.Events, result.TurnID)
	if finalMessage == "" {
		finalMessage = extractFinalAgentTextFromThreadRead(result.ThreadRead, result.TurnID)
	}
	if finalMessage == "" {
		finalMessage = extractFinalAgentTextFromTurnsList(result.TurnsList, result.TurnID)
	}
	if finalMessage == "" {
		return result, fmt.Errorf("app-server turn %s completed without a final agent message", result.TurnID)
	}
	result.FinalMessage = finalMessage
	var payload map[string]any
	if err := json.Unmarshal([]byte(finalMessage), &payload); err != nil {
		return result, fmt.Errorf("app-server final message is not JSON: %w", err)
	}
	if err := validatePayloadAgainstSchema(payload, schemaPath); err != nil {
		return result, err
	}
	result.StructuredOutput = payload
	return result, nil
}

func writeAppServerTurnResult(outDir string, result appServerTurnResult) (string, appServerTurnResult, error) {
	dir := filepath.Join(outDir, "agent_runs")
	if err := ensureSecureDir(dir); err != nil {
		return "", result, err
	}
	eventPath := filepath.Join(dir, result.Stage+"_appserver_events.jsonl")
	if len(result.Events) > 0 {
		f, err := os.OpenFile(eventPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return "", result, err
		}
		enc := json.NewEncoder(f)
		for _, event := range result.Events {
			if err := enc.Encode(redactSecretsInAny(event)); err != nil {
				_ = f.Close()
				return "", result, err
			}
		}
		if err := f.Close(); err != nil {
			return "", result, err
		}
		result.EventLog = filepath.ToSlash(eventPath)
	}
	path := filepath.Join(dir, result.Stage+"_appserver_turn.json")
	return path, result, secureWriteJSON(path, result)
}

func readJSONSchemaObject(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	return schema, nil
}

func appServerCapabilitySnapshot(deckAbs string, startThread bool) (map[string]any, error) {
	if startThread {
		run, err := startAppServerWorkflowRun(deckAbs)
		if err != nil {
			return nil, err
		}
		defer run.close()
		return run.snapshot, nil
	}
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
	if err := client.notify("initialized", nil); err != nil {
		return snapshot, err
	}
	snapshot["initialized"] = true
	for _, method := range []string{"model/list", "experimentalFeature/list", "mcpServerStatus/list"} {
		resp, events, err := client.request(method, map[string]any{}, 20*time.Second)
		addEvents(events)
		if err != nil {
			return snapshot, err
		}
		snapshot[strings.ReplaceAll(method, "/", "_")] = resp["result"]
	}
	snapshot["protocolBundle"] = filepath.ToSlash(filepath.Join("internal", "codex", "protocol", "codex-cli-"+requiredCodexVersion))
	return snapshot, nil
}

func appServerThreadFeatureProbe(threadID string) (map[string]any, error) {
	client, err := newAppServerClient()
	if err != nil {
		return nil, err
	}
	defer client.close()
	snapshot := map[string]any{
		"schemaVersion": "slidex.threadFeatureProbe.v1",
		"generatedAt":   time.Now().UTC().Format(time.RFC3339),
		"codexVersion":  installedCodexVersion(),
		"threadId":      threadID,
		"events":        []map[string]any{},
	}
	initResp, events, err := client.request("initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "slidex", "title": "slidex CLI", "version": toolVersion},
		"capabilities": map[string]any{"experimentalApi": true},
	}, 10*time.Second)
	snapshot["events"] = events
	if err != nil {
		return snapshot, err
	}
	snapshot["initialize"] = initResp["result"]
	if err := client.notify("initialized", nil); err != nil {
		return snapshot, err
	}
	snapshot["initialized"] = true
	resumeResp, events, err := client.request("thread/resume", map[string]any{
		"threadId":       threadID,
		"cwd":            mustAbs("."),
		"approvalPolicy": "never",
		"sandbox":        "read-only",
		"excludeTurns":   true,
	}, 20*time.Second)
	if len(events) > 0 {
		existing, _ := snapshot["events"].([]map[string]any)
		snapshot["events"] = append(existing, events...)
	}
	if err != nil {
		return snapshot, err
	}
	snapshot["thread_resume"] = resumeResp["result"]
	resp, events, err := client.request("experimentalFeature/list", map[string]any{"threadId": threadID}, 20*time.Second)
	if len(events) > 0 {
		existing, _ := snapshot["events"].([]map[string]any)
		snapshot["events"] = append(existing, events...)
	}
	if err != nil {
		return snapshot, err
	}
	snapshot["experimentalFeature_thread_scoped"] = resp["result"]
	return snapshot, nil
}

func extractThreadID(v any) string {
	obj, _ := v.(map[string]any)
	thread, _ := obj["thread"].(map[string]any)
	id, _ := thread["id"].(string)
	return id
}

func turnIDFromNotification(method string, params map[string]any) string {
	switch method {
	case "turn/started":
		turn, _ := params["turn"].(map[string]any)
		id, _ := turn["id"].(string)
		return id
	case "item/started", "item/completed":
		id, _ := params["turnId"].(string)
		return id
	default:
		return ""
	}
}

func extractTurnID(v any) string {
	obj, _ := v.(map[string]any)
	turn, _ := obj["turn"].(map[string]any)
	id, _ := turn["id"].(string)
	return id
}

func turnStatus(v any) string {
	params, _ := v.(map[string]any)
	turn, _ := params["turn"].(map[string]any)
	status, _ := turn["status"].(string)
	return status
}

func turnIDFromCompletion(v any) string {
	params, _ := v.(map[string]any)
	turn, _ := params["turn"].(map[string]any)
	id, _ := turn["id"].(string)
	return id
}

func turnError(v any) any {
	params, _ := v.(map[string]any)
	turn, _ := params["turn"].(map[string]any)
	return turn["error"]
}

func extractFinalAgentTextFromEvents(events []map[string]any, turnID string) string {
	best := ""
	for _, event := range events {
		method, _ := event["method"].(string)
		if method != "item/completed" {
			continue
		}
		params, _ := event["params"].(map[string]any)
		if gotTurnID, _ := params["turnId"].(string); gotTurnID != "" && gotTurnID != turnID {
			continue
		}
		if text := extractAgentTextFromItem(params["item"]); text != "" {
			best = text
		}
	}
	return best
}

func extractFinalAgentTextFromThreadRead(v any, turnID string) string {
	result, _ := v.(map[string]any)
	thread, _ := result["thread"].(map[string]any)
	return extractFinalAgentTextFromTurns(thread["turns"], turnID)
}

func extractFinalAgentTextFromTurnsList(v any, turnID string) string {
	result, _ := v.(map[string]any)
	return extractFinalAgentTextFromTurns(result["data"], turnID)
}

func extractFinalAgentTextFromTurns(v any, turnID string) string {
	turns, _ := v.([]any)
	best := ""
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if id, _ := turn["id"].(string); id != "" && id != turnID {
			continue
		}
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			if text := extractAgentTextFromItem(rawItem); text != "" {
				best = text
			}
		}
	}
	return best
}

func extractAgentTextFromItem(v any) string {
	item, _ := v.(map[string]any)
	if itemType, _ := item["type"].(string); itemType != "agentMessage" {
		return ""
	}
	text, _ := item["text"].(string)
	if text == "" {
		return ""
	}
	phase, _ := item["phase"].(string)
	if phase == "" || phase == "final_answer" {
		return text
	}
	return ""
}

func appServerGoalRequest(deckAbs, threadID, method string, params map[string]any) (map[string]any, string, []map[string]any, error) {
	client, err := newAppServerClient()
	if err != nil {
		return nil, threadID, nil, err
	}
	defer client.close()
	var allEvents []map[string]any
	initResp, events, err := client.request("initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "slidex", "title": "slidex CLI", "version": toolVersion},
		"capabilities": map[string]any{"experimentalApi": true},
	}, 10*time.Second)
	_ = initResp
	allEvents = append(allEvents, events...)
	if err != nil {
		return nil, threadID, allEvents, err
	}
	if err := client.notify("initialized", nil); err != nil {
		return nil, threadID, allEvents, err
	}
	if threadID == "" || threadID == "local-mirror" {
		resp, events, err := client.request("thread/start", map[string]any{
			"cwd":                   mustAbs("."),
			"approvalPolicy":        "never",
			"sandbox":               "read-only",
			"serviceName":           "slidex-goal",
			"model":                 defaultCodexModel(),
			"runtimeWorkspaceRoots": uniqueStrings([]string{mustAbs("."), deckAbs}),
		}, 20*time.Second)
		allEvents = append(allEvents, events...)
		if err != nil {
			return nil, threadID, allEvents, err
		}
		threadID = extractThreadID(resp["result"])
	} else {
		resp, events, err := client.request("thread/resume", map[string]any{
			"threadId":       threadID,
			"cwd":            mustAbs("."),
			"approvalPolicy": "never",
			"sandbox":        "read-only",
			"excludeTurns":   true,
		}, 20*time.Second)
		_ = resp
		allEvents = append(allEvents, events...)
		if err != nil {
			return nil, threadID, allEvents, err
		}
	}
	if params == nil {
		params = map[string]any{}
	}
	params["threadId"] = threadID
	resp, events, err := client.request(method, params, 20*time.Second)
	allEvents = append(allEvents, events...)
	if err != nil {
		return resp, threadID, allEvents, err
	}
	result, _ := resp["result"].(map[string]any)
	return result, threadID, allEvents, nil
}

func appServerReadThread(threadID string) (map[string]any, error) {
	client, err := newAppServerClient()
	if err != nil {
		return nil, err
	}
	defer client.close()
	if _, _, err := client.request("initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "slidex", "title": "slidex CLI", "version": toolVersion},
		"capabilities": map[string]any{"experimentalApi": true},
	}, 10*time.Second); err != nil {
		return nil, err
	}
	if err := client.notify("initialized", nil); err != nil {
		return nil, err
	}
	if _, _, err := client.request("thread/resume", map[string]any{
		"threadId":       threadID,
		"cwd":            mustAbs("."),
		"approvalPolicy": "never",
		"sandbox":        "read-only",
		"excludeTurns":   true,
	}, 20*time.Second); err != nil {
		return nil, err
	}
	resp, _, err := client.request("thread/read", map[string]any{"threadId": threadID, "includeTurns": true}, 20*time.Second)
	if err != nil {
		return nil, err
	}
	result, _ := resp["result"].(map[string]any)
	return result, nil
}
