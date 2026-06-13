package main

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

const (
	appServerPluginSmokeName = "app_server_plugin_smoke.json"
	appServerSkillSmokeName  = "app_server_skill_smoke.json"

	appServerLineBuffer           = 256
	maxAppServerJSONLineBytes     = maxCodexMessageBytes
	maxAppServerStderrBytes       = maxDeckLogBytes
	maxAppServerNotifications     = 1024
	maxAppServerNotificationBytes = maxDeckLogBytes
)

type appServerClient struct {
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	lines         chan map[string]any
	nextID        int
	stderr        *synchronizedLimitedBuffer
	protocolErrMu sync.Mutex
	protocolErr   error
	stage         string
}

type appServerWorkflowRun struct {
	client   *appServerClient
	deckAbs  string
	outDir   string
	threadID string
	snapshot map[string]any
}

type synchronizedLimitedBuffer struct {
	mu  sync.Mutex
	buf limitedOutputBuffer
}

func newSynchronizedLimitedBuffer(maxBytes int64) *synchronizedLimitedBuffer {
	return &synchronizedLimitedBuffer{buf: limitedOutputBuffer{maxBytes: maxBytes}}
}

func (b *synchronizedLimitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *synchronizedLimitedBuffer) String(label string) string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	text := string(b.buf.Bytes())
	if err := b.buf.Err(label); err != nil {
		if strings.TrimSpace(text) == "" {
			return err.Error()
		}
		return strings.TrimRight(text, "\n") + "\n[" + err.Error() + "]"
	}
	return text
}

type appServerNotificationCollector struct {
	items    []map[string]any
	bytes    int64
	maxCount int
	maxBytes int64
}

func newAppServerNotificationCollector(maxCount int, maxBytes int64) *appServerNotificationCollector {
	return &appServerNotificationCollector{maxCount: maxCount, maxBytes: maxBytes}
}

func (c *appServerNotificationCollector) append(msg map[string]any) error {
	if _, hasMethod := msg["method"]; !hasMethod {
		return nil
	}
	if c.maxCount > 0 && len(c.items) >= c.maxCount {
		return fmt.Errorf("app-server notification count exceeded maximum allowed size: %d > %d", len(c.items)+1, c.maxCount)
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("app-server notification cannot be measured: %w", err)
	}
	size := int64(len(raw))
	if c.maxBytes > 0 && c.bytes > c.maxBytes-size {
		return fmt.Errorf("app-server notification bytes exceeded maximum allowed size: %d bytes > %d", c.bytes+size, c.maxBytes)
	}
	c.items = append(c.items, msg)
	c.bytes += size
	return nil
}

func (c *appServerNotificationCollector) list() []map[string]any {
	if c == nil {
		return nil
	}
	return c.items
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

type appServerPluginSmokeResult struct {
	SchemaVersion            string         `json:"schemaVersion"`
	ToolName                 string         `json:"toolName"`
	ToolVersion              string         `json:"toolVersion"`
	Status                   string         `json:"status"`
	GeneratedAt              string         `json:"generatedAt"`
	CodexVersion             string         `json:"codexVersion"`
	Workspace                string         `json:"workspace"`
	DeckID                   string         `json:"deckId"`
	ThreadID                 string         `json:"threadId"`
	MarketplacePath          string         `json:"marketplacePath"`
	PluginReadOK             bool           `json:"pluginReadOk"`
	PluginInstallStateFound  bool           `json:"pluginInstallStateFound"`
	PluginInstalled          bool           `json:"pluginInstalled"`
	PluginEnabled            bool           `json:"pluginEnabled"`
	PluginVersion            string         `json:"pluginVersion,omitempty"`
	PluginPath               string         `json:"pluginPath,omitempty"`
	StartSkillFound          bool           `json:"startSkillFound"`
	StartSkillPath           string         `json:"startSkillPath,omitempty"`
	MCPServerFound           bool           `json:"mcpServerFound"`
	WorkbenchToolsFound      []string       `json:"workbenchToolsFound"`
	WorkbenchURL             string         `json:"workbenchUrl,omitempty"`
	ServerBind               string         `json:"serverBind,omitempty"`
	StartStatus              string         `json:"startStatus,omitempty"`
	StatusStatus             string         `json:"statusStatus,omitempty"`
	StopStatus               string         `json:"stopStatus,omitempty"`
	EvidencePath             string         `json:"evidencePath,omitempty"`
	BrowserOpenStrategy      string         `json:"browserOpenStrategy,omitempty"`
	ProprietaryCanvasAPI     string         `json:"proprietaryCanvasApi,omitempty"`
	RestartRequiredBefore    bool           `json:"restartRequiredBefore"`
	RestartRequiredAfter     bool           `json:"restartRequiredAfter"`
	PluginVerificationStatus string         `json:"pluginVerificationStatus,omitempty"`
	Checks                   map[string]any `json:"checks"`
}

type appServerSkillSmokeResult struct {
	SchemaVersion                   string              `json:"schemaVersion"`
	ToolName                        string              `json:"toolName"`
	ToolVersion                     string              `json:"toolVersion"`
	Status                          string              `json:"status"`
	Error                           string              `json:"error,omitempty"`
	GeneratedAt                     string              `json:"generatedAt"`
	CodexVersion                    string              `json:"codexVersion"`
	Workspace                       string              `json:"workspace"`
	DeckID                          string              `json:"deckId"`
	DeckDir                         string              `json:"deckDir"`
	ThreadID                        string              `json:"threadId,omitempty"`
	TurnID                          string              `json:"turnId,omitempty"`
	TurnStatus                      string              `json:"turnStatus,omitempty"`
	SkillName                       string              `json:"skillName"`
	SkillPath                       string              `json:"skillPath,omitempty"`
	SkillFound                      bool                `json:"skillFound"`
	PluginReadOK                    bool                `json:"pluginReadOk"`
	TurnSandboxPolicy               string              `json:"turnSandboxPolicy"`
	WorkbenchCommand                string              `json:"workbenchCommand"`
	PromptSha256                    string              `json:"promptSha256"`
	FinalMessage                    string              `json:"finalMessage,omitempty"`
	EventCount                      int                 `json:"eventCount"`
	DeckCreated                     bool                `json:"deckCreated"`
	ManifestExists                  bool                `json:"manifestExists"`
	ManifestPath                    string              `json:"manifestPath"`
	EvidencePath                    string              `json:"evidencePath"`
	WorkbenchURL                    string              `json:"workbenchUrl,omitempty"`
	ServerBind                      string              `json:"serverBind,omitempty"`
	SessionID                       string              `json:"sessionId,omitempty"`
	StartStatus                     string              `json:"startStatus,omitempty"`
	DraftStatus                     string              `json:"draftStatus,omitempty"`
	SaveStatus                      string              `json:"saveStatus,omitempty"`
	StopStatus                      string              `json:"stopStatus,omitempty"`
	TokenRedacted                   bool                `json:"tokenRedacted"`
	RawTokenAbsentFromArtifacts     bool                `json:"rawTokenAbsentFromArtifacts"`
	SavedInputVerified              bool                `json:"savedInputVerified"`
	BriefPath                       string              `json:"briefPath,omitempty"`
	DraftPath                       string              `json:"draftPath,omitempty"`
	BrowserOpenStrategy             string              `json:"browserOpenStrategy,omitempty"`
	ProprietaryCanvasAPI            string              `json:"proprietaryCanvasApi"`
	IsActualCodexAppBrowserEvidence bool                `json:"isActualCodexAppBrowserEvidence"`
	VerifiedFiles                   map[string]artifact `json:"verifiedFiles,omitempty"`
	Checks                          map[string]any      `json:"checks"`
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
	client := newAppServerClientState()
	client.stdin = conn
	go client.scanStdout(conn)
	return client, nil
}

func newAppServerClientState() *appServerClient {
	return &appServerClient{
		lines:  make(chan map[string]any, appServerLineBuffer),
		stderr: newSynchronizedLimitedBuffer(maxAppServerStderrBytes),
	}
}

func newAppServerClientCommand(name string, args ...string) (*appServerClient, error) {
	cmd := appServerClientExecCommand(name, args...)
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
	client := newAppServerClientState()
	client.cmd = cmd
	client.stdin = stdin
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go client.scanStdout(stdout)
	go func() {
		_, _ = io.Copy(client.stderr, stderr)
	}()
	return client, nil
}

func appServerClientExecCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	configureManagedAppServerCommand(cmd)
	return cmd
}

func (c *appServerClient) scanStdout(stdout io.Reader) {
	c.scanStdoutWithMaxLineBytes(stdout, maxAppServerJSONLineBytes)
}

func (c *appServerClient) scanStdoutWithMaxLineBytes(stdout io.Reader, maxLineBytes int64) {
	scanner := bufio.NewScanner(stdout)
	maxToken := int(maxLineBytes)
	if maxToken <= 0 {
		maxToken = 1024 * 1024
	}
	initialBuffer := 64 * 1024
	if maxToken < initialBuffer {
		initialBuffer = maxToken
	}
	if initialBuffer <= 0 {
		initialBuffer = 1
	}
	buf := make([]byte, 0, initialBuffer)
	scanner.Buffer(buf, maxToken)
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
	if err := scanner.Err(); err != nil {
		c.setProtocolErr(fmt.Errorf("app-server stdout scan failed: %w", err))
	}
	close(c.lines)
}

func (c *appServerClient) setProtocolErr(err error) {
	if err == nil {
		return
	}
	c.protocolErrMu.Lock()
	defer c.protocolErrMu.Unlock()
	if c.protocolErr == nil {
		c.protocolErr = err
	}
}

func (c *appServerClient) protocolError() error {
	c.protocolErrMu.Lock()
	defer c.protocolErrMu.Unlock()
	return c.protocolErr
}

func (c *appServerClient) stderrText() string {
	return c.stderr.String("app-server stderr")
}

func (c *appServerClient) closedStdoutError(context string) error {
	if err := c.protocolError(); err != nil {
		return fmt.Errorf("%s: %w; stderr: %s", context, err, c.stderrText())
	}
	return fmt.Errorf("%s: %s", context, c.stderrText())
}

func (c *appServerClient) close() {
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd == nil {
		return
	}
	if c.cmd.Process == nil {
		_ = c.cmd.Wait()
		return
	}
	pid := c.cmd.Process.Pid
	done := make(chan struct{})
	go func() {
		_ = c.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
		return
	case <-time.After(2 * time.Second):
		signalManagedProcess(pid)
	}
	select {
	case <-done:
		return
	case <-time.After(3 * time.Second):
		killManagedProcess(pid)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
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
	notifications := newAppServerNotificationCollector(maxAppServerNotifications, maxAppServerNotificationBytes)
	for {
		select {
		case msg, ok := <-c.lines:
			if !ok {
				return nil, notifications.list(), c.closedStdoutError("app-server closed stdout")
			}
			if got, ok := numberAsInt(msg["id"]); ok && got == id {
				if errObj, exists := msg["error"]; exists {
					return msg, notifications.list(), fmt.Errorf("app-server %s error: %v", method, errObj)
				}
				return msg, notifications.list(), nil
			}
			if err := notifications.append(msg); err != nil {
				c.close()
				return nil, notifications.list(), err
			}
		case <-deadline:
			return nil, notifications.list(), fmt.Errorf("app-server %s timed out", method)
		}
	}
}

func (c *appServerClient) waitForTurnCompletion(threadID, turnID string, timeout time.Duration) ([]map[string]any, map[string]any, error) {
	deadline := time.After(timeout)
	notifications := newAppServerNotificationCollector(maxAppServerNotifications, maxAppServerNotificationBytes)
	activeTurnID := turnID
	for {
		select {
		case msg, ok := <-c.lines:
			if !ok {
				return notifications.list(), nil, c.closedStdoutError("app-server closed stdout while waiting for turn completion")
			}
			if err := notifications.append(msg); err != nil {
				c.close()
				return notifications.list(), nil, err
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
					return notifications.list(), params, nil
				}
			}
		case <-deadline:
			return notifications.list(), nil, fmt.Errorf("app-server turn/start timed out waiting for turn %s", turnID)
		}
	}
}

func (c *appServerClient) waitForThreadCompacted(threadID string, timeout time.Duration) ([]map[string]any, map[string]any, error) {
	deadline := time.After(timeout)
	notifications := newAppServerNotificationCollector(maxAppServerNotifications, maxAppServerNotificationBytes)
	for {
		select {
		case msg, ok := <-c.lines:
			if !ok {
				return notifications.list(), nil, c.closedStdoutError("app-server closed stdout while waiting for thread compact")
			}
			if err := notifications.append(msg); err != nil {
				c.close()
				return notifications.list(), nil, err
			}
			method, _ := msg["method"].(string)
			if method != "thread/compacted" {
				continue
			}
			params, _ := msg["params"].(map[string]any)
			if paramsThreadID, _ := params["threadId"].(string); paramsThreadID != "" && paramsThreadID != threadID {
				continue
			}
			return notifications.list(), params, nil
		case <-deadline:
			return notifications.list(), nil, fmt.Errorf("app-server thread/compact/start timed out waiting for thread %s", threadID)
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
	stageComponent := safeFilenameComponent(result.Stage)
	eventPath := filepath.Join(dir, stageComponent+"_appserver_events.jsonl")
	if len(result.Events) > 0 {
		f, err := openSecureTruncateFile(eventPath, 0o600)
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
	path := filepath.Join(dir, stageComponent+"_appserver_turn.json")
	return path, result, secureWriteJSON(path, result)
}

func readJSONSchemaObject(path string) (map[string]any, error) {
	raw, err := readRegularFileWithMaxBytes(path, maxProjectSchemaBytes)
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

func appServerWorkbenchPluginSmoke(workspace, deckID string) (appServerPluginSmokeResult, error) {
	workspace = workspaceRoot(workspace)
	if err := validateDeckID(deckID); err != nil {
		return appServerPluginSmokeResult{}, err
	}
	if err := ensureSmokeWorkspaceTemplate(workspace); err != nil {
		return appServerPluginSmokeResult{}, err
	}
	result := appServerPluginSmokeResult{
		SchemaVersion:   "slidex.appServerPluginSmoke.v1",
		ToolName:        toolName,
		ToolVersion:     toolVersion,
		Status:          "fail",
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		CodexVersion:    installedCodexVersion(),
		Workspace:       filepath.ToSlash(workspace),
		DeckID:          deckID,
		MarketplacePath: filepath.ToSlash(mustAbs(filepath.Join(".agents", "plugins", "marketplace.json"))),
		Checks:          map[string]any{},
	}
	client, err := newAppServerClient()
	if err != nil {
		return result, err
	}
	defer client.close()
	if _, _, err := client.request("initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "slidex", "title": "slidex CLI", "version": toolVersion},
		"capabilities": map[string]any{"experimentalApi": true},
	}, 10*time.Second); err != nil {
		return result, err
	}
	if err := client.notify("initialized", nil); err != nil {
		return result, err
	}
	pluginResp, _, err := client.request("plugin/read", map[string]any{
		"pluginName":      "slidex",
		"marketplacePath": mustAbs(filepath.Join(".agents", "plugins", "marketplace.json")),
	}, 20*time.Second)
	if err != nil {
		return result, err
	}
	result.PluginReadOK = pluginResp["result"] != nil
	result.PluginVersion, result.PluginPath = pluginReadVersionAndPath(pluginResp["result"])
	result.PluginInstalled, result.PluginEnabled, result.PluginInstallStateFound = pluginReadInstallState(pluginResp["result"], toolName)
	if result.PluginPath != "" && !filepath.IsAbs(result.PluginPath) {
		result.PluginPath = mustAbs(result.PluginPath)
	}
	result.PluginPath = filepath.ToSlash(result.PluginPath)
	result.Checks["pluginRead"] = summarizeJSONForEvidence(pluginResp["result"])
	result.Checks["pluginInstallState"] = map[string]any{
		"found":     result.PluginInstallStateFound,
		"installed": result.PluginInstalled,
		"enabled":   result.PluginEnabled,
	}

	skillsResp, _, err := client.request("skills/list", map[string]any{"cwds": []string{mustAbs(".")}, "forceReload": true}, 20*time.Second)
	if err != nil {
		return result, err
	}
	startSkillPath, startSkillFound := findSkillPathInSkillsList(skillsResp["result"], "slidex:slidex-start")
	if !startSkillFound {
		startSkillPath, startSkillFound = findSkillPathInSkillsList(skillsResp["result"], "slidex-start")
	}
	if startSkillFound && !filepath.IsAbs(startSkillPath) {
		startSkillPath = mustAbs(startSkillPath)
	}
	result.StartSkillFound = startSkillFound || jsonContainsString(skillsResp["result"], "slidex-start")
	result.StartSkillPath = filepath.ToSlash(startSkillPath)
	result.Checks["skillsList"] = map[string]any{
		"containsSlidexStart": result.StartSkillFound,
		"startSkillPath":      result.StartSkillPath,
	}

	threadResp, _, err := client.request("thread/start", map[string]any{
		"cwd":                   mustAbs("."),
		"approvalPolicy":        "never",
		"sandbox":               "read-only",
		"serviceName":           "slidex-plugin-smoke",
		"model":                 defaultCodexModel(),
		"runtimeWorkspaceRoots": uniqueStrings([]string{mustAbs("."), workspace}),
	}, 20*time.Second)
	if err != nil {
		return result, err
	}
	result.ThreadID = extractThreadID(threadResp["result"])
	if result.ThreadID == "" {
		return result, errors.New("app-server thread/start did not return a thread id")
	}

	mcpResp, _, err := client.request("mcpServerStatus/list", map[string]any{"threadId": result.ThreadID, "detail": "toolsAndAuthOnly"}, 20*time.Second)
	if err != nil {
		return result, err
	}
	result.MCPServerFound = jsonContainsString(mcpResp["result"], "slidex")
	for _, tool := range []string{"workbench.start", "workbench.status", "workbench.stop"} {
		if jsonContainsString(mcpResp["result"], tool) {
			result.WorkbenchToolsFound = append(result.WorkbenchToolsFound, tool)
		}
	}
	result.Checks["mcpServerStatus"] = map[string]any{"containsSlidex": result.MCPServerFound, "workbenchToolsFound": result.WorkbenchToolsFound}

	previousStage := client.stage
	client.stage = "plugin_smoke"
	defer func() { client.stage = previousStage }()
	startResp, _, err := client.request("mcpServer/tool/call", appServerWorkbenchToolCallParams(result.ThreadID, "workbench.start", workspace, deckID), 45*time.Second)
	if err != nil {
		return result, err
	}
	startContent := structuredContentFromMCPToolCall(startResp)
	result.StartStatus, _ = startContent["status"].(string)
	if workbench, _ := startContent["workbench"].(map[string]any); workbench != nil {
		if result.StartStatus == "" {
			result.StartStatus, _ = workbench["status"].(string)
		}
		result.WorkbenchURL, _ = workbench["url"].(string)
		result.ServerBind, _ = workbench["serverBind"].(string)
		result.BrowserOpenStrategy, _ = workbench["browserOpenStrategy"].(string)
	}
	result.ProprietaryCanvasAPI, _ = startContent["proprietaryCanvasAPI"].(string)
	result.Checks["workbenchStart"] = summarizeJSONForEvidence(startContent)

	statusResp, _, err := client.request("mcpServer/tool/call", appServerWorkbenchToolCallParams(result.ThreadID, "workbench.status", workspace, deckID), 20*time.Second)
	if err != nil {
		_, _ = appServerWorkbenchStop(client, result.ThreadID, workspace, deckID)
		return result, err
	}
	statusContent := structuredContentFromMCPToolCall(statusResp)
	if statusWorkbench, _ := statusContent["workbench"].(map[string]any); statusWorkbench != nil {
		result.StatusStatus, _ = statusWorkbench["status"].(string)
	} else {
		result.StatusStatus, _ = statusContent["status"].(string)
	}
	result.Checks["workbenchStatus"] = summarizeJSONForEvidence(statusContent)

	stopContent, stopErr := appServerWorkbenchStop(client, result.ThreadID, workspace, deckID)
	if stopErr != nil {
		return result, stopErr
	}
	if stopContent != nil {
		result.StopStatus, _ = stopContent["status"].(string)
		result.Checks["workbenchStop"] = summarizeJSONForEvidence(stopContent)
	}
	if result.PluginReadOK && result.StartSkillFound && result.MCPServerFound &&
		containsAllStrings(result.WorkbenchToolsFound, []string{"workbench.start", "workbench.status", "workbench.stop"}) &&
		result.StartStatus == "running" && result.StatusStatus == "running" && result.StopStatus == "stopped" &&
		result.ServerBind == "127.0.0.1" && result.ProprietaryCanvasAPI == "not_used" {
		result.Status = "pass"
	}
	applyPostRestartPluginVerification(&result)
	deckAbs := filepath.Join(workspace, "decks", deckID)
	evidencePath := filepath.Join(deckAbs, "out", appServerPluginSmokeName)
	result.EvidencePath = filepath.ToSlash(evidencePath)
	if err := secureWriteJSON(evidencePath, result); err != nil {
		return result, err
	}
	return result, nil
}

func appServerWorkbenchSkillSmoke(workspace, deckID string) (result appServerSkillSmokeResult, err error) {
	workspace = workspaceRoot(workspace)
	if err := validateDeckID(deckID); err != nil {
		return appServerSkillSmokeResult{}, err
	}
	deckAbs := filepath.Join(workspace, "decks", deckID)
	manifestPath := filepath.Join(deckAbs, "out", workbenchManifestName)
	evidencePath := filepath.Join(deckAbs, "out", appServerSkillSmokeName)
	command := appServerSkillSmokeWorkbenchCommand(workspace, deckID)
	prompt := appServerSkillSmokePrompt(workspace, deckID, command)
	result = appServerSkillSmokeResult{
		SchemaVersion:                   "slidex.appServerSkillSmoke.v1",
		ToolName:                        toolName,
		ToolVersion:                     toolVersion,
		Status:                          "fail",
		GeneratedAt:                     time.Now().UTC().Format(time.RFC3339),
		CodexVersion:                    installedCodexVersion(),
		Workspace:                       filepath.ToSlash(workspace),
		DeckID:                          deckID,
		DeckDir:                         filepath.ToSlash(deckAbs),
		SkillName:                       "slidex:slidex-start",
		TurnSandboxPolicy:               "dangerFullAccess",
		WorkbenchCommand:                command,
		PromptSha256:                    sha256Bytes([]byte(prompt)),
		ManifestPath:                    filepath.ToSlash(manifestPath),
		EvidencePath:                    filepath.ToSlash(evidencePath),
		BriefPath:                       filepath.ToSlash(filepath.Join(deckAbs, "brief.md")),
		DraftPath:                       filepath.ToSlash(filepath.Join(deckAbs, "out", workbenchDraftName)),
		ProprietaryCanvasAPI:            "not_used",
		IsActualCodexAppBrowserEvidence: false,
		VerifiedFiles:                   map[string]artifact{},
		Checks:                          map[string]any{"actualCodexAppBrowserEvidence": false, "turnSandboxPolicy": "dangerFullAccess"},
	}
	defer func() {
		if stopErr := finalizeAppServerSkillSmoke(workspace, deckID, deckAbs, &result); stopErr != nil {
			result.Checks["workbenchStopError"] = stopErr.Error()
			if err == nil {
				err = stopErr
			}
		}
		if err != nil {
			result.Error = err.Error()
		}
		result.Status = appServerSkillSmokeStatus(result)
		if writeErr := secureWriteJSON(evidencePath, result); writeErr != nil && err == nil {
			err = writeErr
		}
	}()

	client, err := newAppServerClient()
	if err != nil {
		return result, err
	}
	defer client.close()
	if _, _, err := client.request("initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "slidex", "title": "slidex CLI", "version": toolVersion},
		"capabilities": map[string]any{"experimentalApi": true},
	}, 10*time.Second); err != nil {
		return result, err
	}
	if err := client.notify("initialized", nil); err != nil {
		return result, err
	}
	pluginResp, _, err := client.request("plugin/read", map[string]any{
		"pluginName":      "slidex",
		"marketplacePath": mustAbs(filepath.Join(".agents", "plugins", "marketplace.json")),
	}, 20*time.Second)
	if err != nil {
		return result, err
	}
	result.PluginReadOK = pluginResp["result"] != nil
	result.Checks["pluginRead"] = summarizeJSONForEvidence(pluginResp["result"])

	skillsResp, _, err := client.request("skills/list", map[string]any{"cwds": []string{mustAbs(".")}, "forceReload": true}, 20*time.Second)
	if err != nil {
		return result, err
	}
	skillPath, found := findSkillPathInSkillsList(skillsResp["result"], result.SkillName)
	if found && !filepath.IsAbs(skillPath) {
		skillPath = mustAbs(skillPath)
	}
	result.SkillFound = found
	result.SkillPath = filepath.ToSlash(skillPath)
	result.Checks["skillsList"] = map[string]any{
		"containsSkill": result.SkillFound,
		"skillName":     result.SkillName,
		"skillPath":     result.SkillPath,
	}
	if !result.SkillFound {
		return result, fmt.Errorf("installed Codex skill %q was not found in skills/list", result.SkillName)
	}

	threadResp, _, err := client.request("thread/start", map[string]any{
		"cwd":                   mustAbs("."),
		"approvalPolicy":        "never",
		"sandbox":               "workspace-write",
		"serviceName":           "slidex-skill-smoke",
		"model":                 defaultCodexModel(),
		"runtimeWorkspaceRoots": uniqueStrings([]string{mustAbs("."), workspace}),
	}, 20*time.Second)
	if err != nil {
		return result, err
	}
	result.ThreadID = extractThreadID(threadResp["result"])
	if result.ThreadID == "" {
		return result, errors.New("app-server thread/start did not return a thread id")
	}

	input := []map[string]any{
		{"type": "skill", "name": result.SkillName, "path": skillPath},
		{"type": "text", "text": prompt},
	}
	turnResp, events, err := client.request("turn/start", map[string]any{
		"threadId":              result.ThreadID,
		"cwd":                   mustAbs("."),
		"approvalPolicy":        "never",
		"sandboxPolicy":         map[string]any{"type": result.TurnSandboxPolicy},
		"model":                 defaultCodexModel(),
		"runtimeWorkspaceRoots": uniqueStrings([]string{mustAbs("."), workspace}),
		"input":                 input,
	}, 30*time.Second)
	result.EventCount += len(events)
	result.Checks["turnStartEvents"] = summarizeAppServerEventsForEvidence(events)
	if err != nil {
		return result, err
	}
	result.TurnID = extractTurnID(turnResp["result"])
	result.Checks["turnStart"] = summarizeJSONForEvidence(turnResp["result"])
	if result.TurnID == "" {
		return result, errors.New("app-server turn/start did not return a turn id")
	}
	events, completion, err := client.waitForTurnCompletion(result.ThreadID, result.TurnID, 6*time.Minute)
	result.EventCount += len(events)
	result.Checks["turnEvents"] = summarizeAppServerEventsForEvidence(events)
	result.Checks["turnCompletion"] = summarizeJSONForEvidence(completion)
	if err != nil {
		return result, err
	}
	if actualTurnID := turnIDFromCompletion(completion); actualTurnID != "" {
		result.TurnID = actualTurnID
	}
	result.TurnStatus = turnStatus(completion)
	if result.TurnStatus != "completed" {
		return result, fmt.Errorf("app-server skill smoke turn %s did not complete successfully: status=%s error=%v", result.TurnID, result.TurnStatus, turnError(completion))
	}

	readResp, readEvents, readErr := client.request("thread/read", map[string]any{"threadId": result.ThreadID, "includeTurns": true}, 20*time.Second)
	result.EventCount += len(readEvents)
	if readErr != nil {
		result.Checks["threadReadError"] = readErr.Error()
	} else {
		result.FinalMessage = limitEvidenceString(extractFinalAgentTextFromThreadRead(readResp["result"], result.TurnID), 1200)
		result.Checks["threadRead"] = map[string]any{"available": true, "finalMessageFound": result.FinalMessage != ""}
	}
	turnsResp, turnsEvents, turnsErr := client.request("thread/turns/list", map[string]any{"threadId": result.ThreadID, "itemsView": "full", "limit": 20, "sortDirection": "desc"}, 20*time.Second)
	result.EventCount += len(turnsEvents)
	if turnsErr != nil {
		result.Checks["turnsListError"] = turnsErr.Error()
	} else {
		if result.FinalMessage == "" {
			result.FinalMessage = limitEvidenceString(extractFinalAgentTextFromTurnsList(turnsResp["result"], result.TurnID), 1200)
		}
		result.Checks["turnsList"] = map[string]any{"available": true, "finalMessageFound": result.FinalMessage != ""}
	}

	if info, statErr := os.Stat(deckAbs); statErr == nil && info.IsDir() {
		result.DeckCreated = true
	}
	manifest, statusErr := workbenchStatus(workspace, deckID, "")
	if statusErr != nil {
		return result, statusErr
	}
	result.ManifestExists = manifest.URL != ""
	result.StartStatus = manifest.Status
	result.WorkbenchURL = manifest.URL
	result.ServerBind = manifest.ServerBind
	result.SessionID = manifest.SessionID
	result.TokenRedacted = manifest.TokenRedacted
	result.BrowserOpenStrategy = manifest.BrowserOpenStrategy
	result.Checks["workbenchManifestBeforeStop"] = summarizeJSONForEvidence(manifest)
	saveResult, saveErr := saveAppServerSkillSmokeInput(deckAbs, manifest)
	result.DraftStatus = saveResult.DraftStatus
	result.SaveStatus = saveResult.SaveStatus
	result.RawTokenAbsentFromArtifacts = saveResult.RawTokenAbsentFromArtifacts
	result.SavedInputVerified = saveResult.Status == "pass"
	result.VerifiedFiles = saveResult.VerifiedFiles
	result.Checks["workbenchSave"] = summarizeJSONForEvidence(saveResult)
	if saveErr != nil {
		return result, saveErr
	}
	if !result.SavedInputVerified {
		return result, fmt.Errorf("app-server skill smoke workbench save did not pass: status=%s findings=%v", saveResult.Status, saveResult.Findings)
	}
	return result, nil
}

func finalizeAppServerSkillSmoke(workspace, deckID, deckAbs string, result *appServerSkillSmokeResult) error {
	manifest, ok := readWorkbenchManifest(deckAbs)
	if !ok {
		return nil
	}
	result.ManifestExists = true
	if result.WorkbenchURL == "" {
		result.WorkbenchURL = manifest.URL
	}
	if result.ServerBind == "" {
		result.ServerBind = manifest.ServerBind
	}
	if result.SessionID == "" {
		result.SessionID = manifest.SessionID
	}
	if !result.TokenRedacted {
		result.TokenRedacted = manifest.TokenRedacted
	}
	if result.BrowserOpenStrategy == "" {
		result.BrowserOpenStrategy = manifest.BrowserOpenStrategy
	}
	stopped, err := stopWorkbench(workspace, deckID, "")
	if err != nil {
		return err
	}
	result.StopStatus = stopped.Status
	result.Checks["workbenchStop"] = summarizeJSONForEvidence(stopped)
	return nil
}

func appServerSkillSmokeStatus(result appServerSkillSmokeResult) string {
	if result.PluginReadOK &&
		result.SkillFound &&
		result.TurnSandboxPolicy == "dangerFullAccess" &&
		result.TurnStatus == "completed" &&
		result.DeckCreated &&
		result.ManifestExists &&
		result.StartStatus == "running" &&
		result.DraftStatus == "draft_saved" &&
		result.SaveStatus == "saved" &&
		result.SavedInputVerified &&
		result.StopStatus == "stopped" &&
		result.ServerBind == "127.0.0.1" &&
		result.TokenRedacted &&
		result.RawTokenAbsentFromArtifacts &&
		result.ProprietaryCanvasAPI == "not_used" &&
		!result.IsActualCodexAppBrowserEvidence &&
		len(result.VerifiedFiles) == 3 &&
		isLoopbackWorkbenchURL(result.WorkbenchURL) {
		return "pass"
	}
	return "fail"
}

func saveAppServerSkillSmokeInput(deckAbs string, manifest workbenchManifest) (workbenchSaveSmokeResult, error) {
	input := normalizeWorkbenchInput(workbenchSaveInput{
		Title:              "App Server skill smoke",
		Audience:           "Codex App verification reviewer",
		DecisionGoal:       "Verify the slidex-start plugin path can persist initial deck creation input.",
		SourceNotes:        "Generated by slidex codex app-server skill-smoke. This is not Codex App GUI/browser evidence.",
		OutputExpectations: "Deck-local brief.md, out/workbench_draft.json, and out/workbench_manifest.json are current after the skill-started workbench save.",
	})
	briefPath := filepath.Join(deckAbs, "brief.md")
	draftPath := filepath.Join(deckAbs, "out", workbenchDraftName)
	manifestPath := filepath.Join(deckAbs, "out", workbenchManifestName)
	logPath := filepath.Join(deckAbs, "out", "workbench_server.log")
	result := workbenchSaveSmokeResult{
		SchemaVersion:                   "slidex.workbenchSaveSmoke.v1",
		ToolName:                        toolName,
		ToolVersion:                     toolVersion,
		Status:                          "fail",
		GeneratedAt:                     time.Now().UTC().Format(time.RFC3339),
		Workspace:                       manifest.Workspace,
		DeckID:                          manifest.DeckID,
		DeckDir:                         filepath.ToSlash(deckAbs),
		WorkbenchURL:                    manifest.URL,
		SessionID:                       manifest.SessionID,
		ServerBind:                      manifest.ServerBind,
		StartStatus:                     manifest.Status,
		TokenRedacted:                   manifest.TokenRedacted,
		BriefPath:                       filepath.ToSlash(briefPath),
		DraftPath:                       filepath.ToSlash(draftPath),
		ManifestPath:                    filepath.ToSlash(manifestPath),
		LogPath:                         filepath.ToSlash(logPath),
		BrowserOpenStrategy:             manifest.BrowserOpenStrategy,
		IsActualCodexAppBrowserEvidence: false,
		Input:                           input,
		VerifiedFiles:                   map[string]artifact{},
		Checks:                          map[string]any{"actualCodexAppBrowserEvidence": false},
	}
	htmlRaw, err := fetchWorkbenchHTML(manifest.URL)
	if err != nil {
		result.Findings = append(result.Findings, "workbench HTML fetch failed: "+err.Error())
		return result, err
	}
	boot, token, err := extractWorkbenchBoot(string(htmlRaw))
	result.Checks["htmlBytes"] = len(htmlRaw)
	result.Checks["htmlSha256"] = sha256Bytes(htmlRaw)
	if err != nil {
		result.Findings = append(result.Findings, "workbench HTML boot extraction failed: "+err.Error())
		return result, err
	}
	result.HTMLBootstrapTokenFound = token != ""
	result.Checks["htmlBoot"] = map[string]any{
		"deckId":    boot["deckId"],
		"sessionId": boot["sessionId"],
		"apiBase":   boot["apiBase"],
		"tokenHash": sha256Bytes([]byte(token)),
	}
	apiBase, _ := boot["apiBase"].(string)
	if !result.HTMLBootstrapTokenFound || strings.TrimSpace(apiBase) == "" {
		err := errors.New("workbench HTML did not include a usable bootstrap token/apiBase")
		result.Findings = append(result.Findings, err.Error())
		return result, err
	}
	draftResp, err := postWorkbenchJSON(manifest.URL, apiBase+"/draft", token, input)
	if err != nil {
		result.Findings = append(result.Findings, "draft POST failed: "+err.Error())
		return result, err
	}
	result.DraftStatus, _ = draftResp["status"].(string)
	result.Checks["draftResponse"] = summarizeWorkbenchSmokeResponse(draftResp)
	saveResp, err := postWorkbenchJSON(manifest.URL, apiBase+"/save", token, input)
	if err != nil {
		result.Findings = append(result.Findings, "save POST failed: "+err.Error())
		return result, err
	}
	result.SaveStatus, _ = saveResp["status"].(string)
	result.Checks["saveResponse"] = summarizeWorkbenchSmokeResponse(saveResp)

	updated, ok := readWorkbenchManifest(deckAbs)
	if !ok {
		err := fmt.Errorf("workbench manifest missing after save: %s", filepath.ToSlash(manifestPath))
		result.Findings = append(result.Findings, err.Error())
		return result, err
	}
	result.TokenRedacted = updated.TokenRedacted
	result.ServerBind = updated.ServerBind
	result.WorkbenchURL = updated.URL
	result.SessionID = updated.SessionID
	result.Checks["manifestAfterSave"] = publicWorkbenchStatus(updated)
	result.VerifiedFiles = map[string]artifact{
		"brief":    artifactFromPath(briefPath),
		"draft":    artifactFromPath(draftPath),
		"manifest": artifactFromPath(manifestPath),
	}
	tokenCheckPaths := []string{briefPath, draftPath, manifestPath}
	if pathExists(workbenchControlPath(deckAbs)) {
		tokenCheckPaths = append(tokenCheckPaths, workbenchControlPath(deckAbs))
	}
	if pathExists(logPath) {
		tokenCheckPaths = append(tokenCheckPaths, logPath)
	}
	result.RawTokenAbsentFromArtifacts = rawTokenAbsentFromFiles(token, tokenCheckPaths)
	result.Findings = workbenchSaveSmokeFindings(result, updated)
	if result.DraftStatus == "draft_saved" &&
		result.SaveStatus == "saved" &&
		result.ServerBind == "127.0.0.1" &&
		result.TokenRedacted &&
		result.HTMLBootstrapTokenFound &&
		result.RawTokenAbsentFromArtifacts &&
		len(result.Findings) == 0 &&
		len(result.VerifiedFiles) == 3 {
		result.Status = "pass"
	}
	return result, nil
}

func appServerSkillSmokeWorkbenchCommand(workspace, deckID string) string {
	return appServerSkillSmokeWorkbenchCommandForOS(runtime.GOOS, workspace, deckID)
}

func appServerSkillSmokeWorkbenchCommandForOS(goos, workspace, deckID string) string {
	if goos == "windows" {
		return windowsPowerShellCommand("slidex", "workbench", "start", "--workspace", workspace, "--deck-id", deckID)
	}
	quote := shellQuote
	return fmt.Sprintf("slidex workbench start --workspace %s --deck-id %s", quote(workspace), quote(deckID))
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, func(r rune) bool {
		return (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && !strings.ContainsRune("@%_+=:,./-", r)
	}) == -1 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func windowsPowerShellCommand(name string, args ...string) string {
	var script strings.Builder
	script.WriteString("$ErrorActionPreference='Stop'; & ")
	script.WriteString(powershellSingleQuote(name))
	for _, arg := range args {
		script.WriteByte(' ')
		script.WriteString(powershellSingleQuote(arg))
	}
	script.WriteString("; exit $LASTEXITCODE")
	return "powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand " + windowsPowerShellEncodedCommand(script.String())
}

func powershellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func windowsPowerShellEncodedCommand(script string) string {
	encoded := utf16.Encode([]rune(script))
	raw := make([]byte, len(encoded)*2)
	for i, v := range encoded {
		binary.LittleEndian.PutUint16(raw[i*2:], v)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func appServerSkillSmokePrompt(workspace, deckID, command string) string {
	return fmt.Sprintf("Use the installed slidex-start skill. Create or select deck %q in workspace %q. Run exactly this command and no other slidex workflow commands: %s. Do not run render, QA, package, workbench evidence, or browser inspection. When the command completes, reply with the workbench URL and deck path only.", deckID, workspace, command)
}

func findSkillPathInSkillsList(v any, skillName string) (string, bool) {
	if obj, ok := v.(map[string]any); ok {
		if name, _ := obj["name"].(string); name == skillName {
			if path := skillPathFromObject(obj); path != "" {
				return path, true
			}
		}
		for _, value := range obj {
			if path, ok := findSkillPathInSkillsList(value, skillName); ok {
				return path, true
			}
		}
	}
	if items, ok := v.([]any); ok {
		for _, item := range items {
			if path, ok := findSkillPathInSkillsList(item, skillName); ok {
				return path, true
			}
		}
	}
	return "", false
}

func skillPathFromObject(obj map[string]any) string {
	for _, key := range []string{"path", "skillPath", "sourcePath", "file"} {
		if value, _ := obj[key].(string); strings.TrimSpace(value) != "" {
			return value
		}
	}
	for _, value := range obj {
		if nested, ok := value.(map[string]any); ok {
			if path := skillPathFromObject(nested); path != "" {
				return path
			}
		}
	}
	return ""
}

func pluginReadVersionAndPath(v any) (version, path string) {
	version = firstJSONTextByKey(v, "version", "pluginVersion", "localVersion")
	path = firstJSONTextByKey(v, "path", "pluginPath", "sourcePath")
	return version, path
}

func pluginReadInstallState(v any, pluginName string) (installed, enabled, found bool) {
	if obj, ok := v.(map[string]any); ok {
		if metadataString(obj["name"]) == pluginName {
			installedValue, hasInstalled := jsonBoolField(obj, "installed")
			enabledValue, hasEnabled := jsonBoolField(obj, "enabled")
			if hasInstalled && hasEnabled {
				return installedValue, enabledValue, true
			}
		}
		for _, value := range obj {
			if installed, enabled, found := pluginReadInstallState(value, pluginName); found {
				return installed, enabled, true
			}
		}
	}
	if items, ok := v.([]any); ok {
		for _, item := range items {
			if installed, enabled, found := pluginReadInstallState(item, pluginName); found {
				return installed, enabled, true
			}
		}
	}
	return false, false, false
}

func jsonBoolField(obj map[string]any, key string) (bool, bool) {
	value, ok := obj[key]
	if !ok {
		return false, false
	}
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		if strings.EqualFold(v, "true") {
			return true, true
		}
		if strings.EqualFold(v, "false") {
			return false, true
		}
	}
	return false, false
}

func firstJSONTextByKey(v any, keys ...string) string {
	keySet := map[string]bool{}
	for _, key := range keys {
		keySet[key] = true
	}
	if obj, ok := v.(map[string]any); ok {
		for key, value := range obj {
			if keySet[key] {
				if text, _ := value.(string); strings.TrimSpace(text) != "" {
					return strings.TrimSpace(text)
				}
			}
		}
		for _, value := range obj {
			if text := firstJSONTextByKey(value, keys...); text != "" {
				return text
			}
		}
	}
	if items, ok := v.([]any); ok {
		for _, item := range items {
			if text := firstJSONTextByKey(item, keys...); text != "" {
				return text
			}
		}
	}
	return ""
}

func applyPostRestartPluginVerification(result *appServerPluginSmokeResult) {
	status, err := currentUpdateStatus("", "")
	if err != nil {
		result.PluginVerificationStatus = "unknown"
		result.Checks["pluginVerificationError"] = err.Error()
		return
	}
	result.RestartRequiredBefore = status.RestartRequired
	result.PluginVerificationStatus = postRestartPluginVerificationStatus(*result, status.InstallRoot)
	if result.Status == "pass" && result.PluginVerificationStatus == "verified" {
		if err := markPluginVerified(status.InstallRoot, result.PluginVersion, result.PluginPath, result.StartSkillPath); err != nil {
			result.Checks["pluginVerificationError"] = err.Error()
		}
	} else if result.PluginVerificationStatus == "drift" {
		if err := markPluginDrift(status.InstallRoot, result.PluginVersion, result.StartSkillPath); err != nil {
			result.Checks["pluginVerificationError"] = err.Error()
		}
	}
	after, err := currentUpdateStatus(status.InstallRoot, status.MetadataPath)
	if err == nil {
		result.RestartRequiredAfter = after.RestartRequired
	}
	result.Checks["postRestartPluginVerification"] = map[string]any{
		"status":                result.PluginVerificationStatus,
		"restartRequiredBefore": result.RestartRequiredBefore,
		"restartRequiredAfter":  result.RestartRequiredAfter,
		"pluginVersion":         result.PluginVersion,
		"pluginPath":            result.PluginPath,
		"pluginInstallState": map[string]any{
			"found":     result.PluginInstallStateFound,
			"installed": result.PluginInstalled,
			"enabled":   result.PluginEnabled,
		},
		"startSkillPath": result.StartSkillPath,
	}
}

func postRestartPluginVerificationStatus(result appServerPluginSmokeResult, installRoot string) string {
	if !result.PluginReadOK || !result.StartSkillFound {
		return "not_verified"
	}
	if !result.PluginInstallStateFound || !result.PluginInstalled || !result.PluginEnabled {
		return "not_verified"
	}
	if pluginVersionBase(result.PluginVersion) != toolVersion {
		return "drift"
	}
	if strings.TrimSpace(result.PluginPath) == "" || strings.TrimSpace(result.StartSkillPath) == "" {
		return "not_verified"
	}
	pluginRoot := filepath.Join(filepath.Clean(installRoot), "plugins", "slidex")
	pluginPath := filepath.Clean(filepath.FromSlash(result.PluginPath))
	skillPath := filepath.Clean(filepath.FromSlash(result.StartSkillPath))
	if !filepath.IsAbs(pluginPath) || !filepath.IsAbs(skillPath) {
		return "not_verified"
	}
	if !pathWithin(pluginRoot, pluginPath) {
		return "drift"
	}
	if status := postRestartSkillPathStatus(pluginRoot, skillPath, result.PluginVersion); status != "verified" {
		return status
	}
	if status := postRestartPluginMetadataStatus(pluginRoot, result.PluginVersion); status != "verified" {
		return status
	}
	return "verified"
}

func postRestartSkillPathStatus(pluginRoot, skillPath, visiblePluginVersion string) string {
	if !strings.HasSuffix(filepath.ToSlash(skillPath), "skills/slidex-start/SKILL.md") {
		return "drift"
	}
	if pathWithin(pluginRoot, skillPath) {
		return "verified"
	}
	cacheRoot := codexPluginRootForSkillPath(skillPath)
	if cacheRoot == "" {
		return "drift"
	}
	return postRestartPluginMetadataStatus(cacheRoot, visiblePluginVersion)
}

func codexPluginRootForSkillPath(skillPath string) string {
	dir := filepath.Clean(filepath.Dir(skillPath))
	for {
		if _, err := os.Stat(filepath.Join(dir, ".codex-plugin", "plugin.json")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func postRestartPluginMetadataStatus(pluginRoot, visiblePluginVersion string) string {
	manifestPath := filepath.Join(pluginRoot, ".codex-plugin", "plugin.json")
	manifest, err := readCandidateJSON(manifestPath)
	if err != nil {
		return "not_verified"
	}
	if got := metadataString(manifest["name"]); got != toolName {
		return "drift"
	}
	manifestVersion := metadataString(manifest["version"])
	if pluginVersionBase(manifestVersion) != toolVersion {
		return "drift"
	}
	if strings.TrimSpace(visiblePluginVersion) != "" && manifestVersion != visiblePluginVersion {
		return "drift"
	}
	lockPath := filepath.Join(pluginRoot, ".codex-plugin", "version-lock.json")
	lock, err := readCandidateJSON(lockPath)
	if err != nil {
		return "not_verified"
	}
	for _, key := range []string{"pluginVersion", "slidexCliVersion"} {
		if got := metadataString(lock[key]); got != toolVersion {
			return "drift"
		}
	}
	if metadataString(lock["requiredCodexCliVersion"]) == "" {
		return "drift"
	}
	return "verified"
}

func isLoopbackWorkbenchURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" && parsed.Hostname() == "127.0.0.1" && strings.HasPrefix(parsed.Path, "/workbench/")
}

func limitEvidenceString(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}

func ensureSmokeWorkspaceTemplate(workspace string) error {
	template := filepath.Join(workspace, "decks", "_template")
	if _, err := os.Stat(template); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	source := filepath.Join(mustAbs("."), "decks", "_template")
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("smoke workspace template missing and source template is unavailable: %s: %w", filepath.ToSlash(source), err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "decks"), 0o755); err != nil {
		return err
	}
	if err := copyDeckTemplateDir(source, template); err != nil {
		_ = os.RemoveAll(template)
		return err
	}
	return nil
}

func appServerWorkbenchToolCallParams(threadID, tool, workspace, deckID string) map[string]any {
	return map[string]any{
		"threadId": threadID,
		"server":   "slidex",
		"tool":     tool,
		"arguments": map[string]any{
			"workspace": workspace,
			"deckId":    deckID,
		},
	}
}

func appServerWorkbenchStop(client *appServerClient, threadID, workspace, deckID string) (map[string]any, error) {
	stopResp, _, err := client.request("mcpServer/tool/call", appServerWorkbenchToolCallParams(threadID, "workbench.stop", workspace, deckID), 20*time.Second)
	if err != nil {
		return nil, err
	}
	return structuredContentFromMCPToolCall(stopResp), nil
}

func structuredContentFromMCPToolCall(resp map[string]any) map[string]any {
	result, _ := resp["result"].(map[string]any)
	content, _ := result["structuredContent"].(map[string]any)
	if content == nil {
		return map[string]any{}
	}
	return content
}

func jsonContainsString(v any, needle string) bool {
	raw, err := json.Marshal(v)
	return err == nil && strings.Contains(string(raw), needle)
}

func containsAllStrings(got, want []string) bool {
	set := map[string]bool{}
	for _, item := range got {
		set[item] = true
	}
	for _, item := range want {
		if !set[item] {
			return false
		}
	}
	return true
}

func summarizeJSONForEvidence(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	redacted := []byte(redactSecrets(string(raw)))
	if len(redacted) > 32*1024 {
		return map[string]any{"sha256": sha256Bytes(raw), "bytes": len(raw), "redacted": true}
	}
	var out any
	if err := json.Unmarshal(redacted, &out); err != nil {
		return map[string]any{"sha256": sha256Bytes(raw), "bytes": len(raw), "redacted": true}
	}
	return out
}

func summarizeAppServerEventsForEvidence(events []map[string]any) map[string]any {
	methods := map[string]int{}
	for _, event := range events {
		method, _ := event["method"].(string)
		if method == "" {
			method = "unknown"
		}
		methods[method]++
	}
	raw, _ := json.Marshal(redactSecretsInAny(events))
	return map[string]any{
		"count":   len(events),
		"methods": methods,
		"bytes":   len(raw),
		"sha256":  sha256Bytes(raw),
	}
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
