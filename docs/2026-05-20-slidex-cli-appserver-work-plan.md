# 2026-05-20 slidex CLI/App Server 작업 계획

Status: Merged CLI implementation plan from the 2026-05-19 design and the 2026-05-20 Codex CLI 0.132.0 supplement.

기준일: 2026-05-20 KST  
기준 Codex CLI: `0.132.0` exact pin  
충돌 해결 규칙: 두 원문이 충돌하면 `docs/2026-05-20-slidex-cli-appserver-design-codex-0.132.0.md`를 우선한다.

## 1. 병합 범위와 최종 판단

이 문서는 다음 두 문서를 하나의 실행 가능한 CLI 작업 계획으로 합친다.

- `docs/2026-05-19-slidex-cli-appserver-design.md`
- `docs/2026-05-20-slidex-cli-appserver-design-codex-0.132.0.md`

05-19 문서에서 유지한 내용:

- `slidex`가 문서 생성 workflow owner가 되어야 한다는 제품 방향
- deterministic Go engine과 Codex agent runtime의 역할 분리
- 기존 prompt 파일을 폐기하지 않고 durable prompt template로 내장한다는 방향
- `delivery_summary`가 `package`보다 먼저 생성되어야 한다는 stage 순서 정정
- 기존 deterministic 명령의 호환성 유지와 `--deck` 중심 workflow 전환
- App Server와 `codex exec` fallback을 stage 성격에 맞게 병행하는 구조
- reviewer loop, goal, subagent/parallel review, MCP, skill/plugin 보조 계층의 필요성

05-20 문서를 우선해 확정한 내용:

- Codex CLI 기준 버전은 `0.130.0`이 아니라 `0.132.0`이다.
- protocol bundle은 `internal/codex/protocol/codex-cli-0.132.0/`을 기준으로 한다.
- version mismatch는 기본 fail이고, `--allow-codex-protocol-mismatch`가 있을 때만 risk를 남기고 진행한다.
- `PermissionProfile`은 App Server required API type으로 전제하지 않는다.
- `codex exec resume --output-schema`를 fallback/resume path에 포함한다.
- App Server visual QA는 original-resolution local image fidelity 보존을 요구하고 검증한다.
- `experimentalFeature/list`는 global probe와 thread-scoped probe를 모두 고려한다.
- companion skill 경로는 `.codex/skills`가 아니라 `.agents/skills/slidex/SKILL.md`다.
- schema refresh는 `--experimental`을 하드코딩하지 않고 help probe 후 사용한다.
- JSON Schema 검증은 자체 부분 구현이 아니라 full validator로 교체한다.
- HTML slide extraction은 regex primary가 아니라 Chrome DOM enumeration primary, Go HTML parser fallback이다.
- Chrome `--no-sandbox`는 기본값에서 제거하고 명시적 fallback에서만 허용한다.
- Phase 0에서 fixture와 regression harness를 먼저 고정한다.

## 2. 제품 목표

`slidex`는 Codex가 프롬프트를 실행할 때 보조로 쓰는 렌더링 CLI가 아니라, HTML-first business document production 전체를 소유하는 로컬 CLI 프로그램이어야 한다.

사용자는 기본 workflow에서 다음 수동 실행을 직접 조합하지 않는다.

```bash
codex exec - < prompts/one_shot_create_business_doc.md
```

대신 다음 명령을 사용한다.

```bash
slidex run --deck decks/<deck_id>
slidex intake --deck decks/<deck_id>
slidex build --deck decks/<deck_id>
slidex qa --deck decks/<deck_id>
slidex package --deck decks/<deck_id>
```

원칙:

- 기존 prompt 파일은 CLI 내부 durable prompt template로 vendoring한다.
- 렌더링, PDF 패키징, manifest freshness, schema validation, artifact validation은 Go deterministic engine이 수행한다.
- intake 질문 생성, strategy 작성, HTML authoring, claim rewrite, visual/design review는 Codex runtime이 수행한다.
- 모든 agent output은 output schema guard와 local full JSON Schema validation을 모두 통과해야 한다.
- 최종 보고는 transcript 기억이 아니라 `${OUT_DIR}` artifacts에 기반한다.
- PPTX는 생성물이나 선택 납품물로 되살리지 않는다.
- 이 저장소는 hosted application이나 SaaS가 아니라 local automation kit이다.

## 3. 핵심 아키텍처

목표 package layout:

```text
cmd/slidex/
  main.go
internal/cli/
  commands.go
  output.go
  exit_codes.go
internal/workspace/
  resolve.go
  inventory.go
  state.go
  lock.go
  secure_write.go
internal/config/
  config.go
  codex.go
  plugin.go
internal/prompts/
  embed.go
  render.go
  templates/*.md
internal/codex/
  runner.go
  exec_runner.go
  appserver_client.go
  appserver_process.go
  appserver_health.go
  remote_control.go
  protocol/
    codex-cli-0.132.0/
      schema/*.json
      protocol_manifest.json
      generated_types.go
      method_constants.go
  events.go
  goals.go
  reviews.go
  subagents.go
  sdk_adapter.go
internal/pipeline/
  graph.go
  stage.go
  intake.go
  strategy.go
  spec.go
  build.go
  revise.go
  finalize.go
internal/render/
  chrome.go
  dom.go
  manifest.go
internal/pdf/
internal/montage/
internal/qa/
  deterministic.go
  visual.go
  report.go
internal/sync/
internal/schema/
  validator.go
  schemas/*.json
internal/report/
internal/mcpserver/
```

핵심 원칙:

- `cmd/slidex/main.go`는 thin entrypoint로 축소한다.
- `internal/render`, `internal/qa`, `internal/sync`, `internal/schema`는 기존 deterministic 기능을 보존하면서 분리한다.
- `internal/codex`는 Runner interface 뒤에 App Server, exec, future SDK adapter를 숨긴다.
- `internal/pipeline`은 stage graph, skip/resume/retry/repair, state transition을 소유한다.

## 4. CLI 명령 표면

기본 명령:

```bash
slidex init <deck_id> [--from-template decks/_template]
slidex doctor [--deck decks/<deck_id>] [--codex] [--render] [--json]
slidex inspect --deck decks/<deck_id> [--write]
slidex intake --deck decks/<deck_id> [--interactive|--answers FILE]
slidex strategy --deck decks/<deck_id>
slidex spec --deck decks/<deck_id>
slidex build --deck decks/<deck_id>
slidex render --deck decks/<deck_id> [--chrome-no-sandbox]
slidex qa --deck decks/<deck_id> [--visual-review codex|manual|none]
slidex revise --deck decks/<deck_id> [--until pass|risk-accepted]
slidex sync-html-edits --deck decks/<deck_id>
slidex finalize --deck decks/<deck_id>
slidex package --deck decks/<deck_id> [--include-logs]
slidex clean --deck decks/<deck_id> [--logs] [--older-than DURATION]
slidex run --deck decks/<deck_id> [--until package|qa|render] [--non-interactive]
```

Codex 통합 명령:

```bash
slidex codex doctor [--json]
slidex codex app-server start [--listen unix://|ws://127.0.0.1:PORT]
slidex codex app-server status [--json]
slidex codex app-server stop [--force]
slidex codex app-server probe [--listen stdio://]
slidex codex schema refresh [--codex-version 0.132.0]
slidex codex models [--json]
slidex codex features [--thread THREAD_ID] [--json]
slidex codex mcp [--json]
slidex codex plugins [--json]
slidex codex threads list [--deck decks/<deck_id>]
slidex codex threads read <thread_id>
slidex codex review --deck decks/<deck_id> [--stage design|html|qa|delivery]
slidex codex remote-control status [--json]
slidex mcp-server [--stdio]
slidex migrate --deck decks/<deck_id> [--from legacy-html-pdf|pptx-first] [--write]
```

Goal 명령:

```bash
slidex goal set --deck decks/<deck_id> --objective "..." [--token-budget N]
slidex goal status --deck decks/<deck_id>
slidex goal pause --deck decks/<deck_id>
slidex goal resume --deck decks/<deck_id>
slidex goal complete --deck decks/<deck_id>
slidex goal clear --deck decks/<deck_id>
```

명령 정책:

- 기존 `render --html ... --pdf ...` 형태는 advanced override로 유지한다.
- 일반 workflow는 `--deck`으로 active workspace와 output path를 계산한다.
- `/goal`은 TUI slash command이고, `slidex goal`은 App Server goal API wrapper 또는 local mirror command다.
- `goal objective`는 non-empty, 4,000자 이하로 검증한다.
- goal continuation은 usage limit 또는 repeated blocker에서 자동 중단한다.

## 5. 표준 산출물과 상태 파일

Per-deck output path는 `${ACTIVE_DECK_DIR}/out/`이다.

문서 production 산출물:

```text
${OUT_DIR}/strategy.md
${OUT_DIR}/deck_spec.json
${OUT_DIR}/final_deck.html
${OUT_DIR}/final_deck.generated_baseline.html
${OUT_DIR}/rendered_slides/*.png
${OUT_DIR}/final_deck.pdf
${OUT_DIR}/render_manifest.json
${OUT_DIR}/qa_montage.png
${OUT_DIR}/qa_report.md
${OUT_DIR}/notes.md
${OUT_DIR}/delivery_summary.md
```

CLI runtime 산출물:

```text
${OUT_DIR}/slidex_state.json
${OUT_DIR}/codex_threads.json
${OUT_DIR}/run_log.jsonl
${OUT_DIR}/agent_runs/<stage>/<thread_id>.json
${OUT_DIR}/agent_reviews/round_01/reviewer_*.json
${OUT_DIR}/agent_reviews/round_01/reviewer_*.md
${OUT_DIR}/agent_reviews/round_01/resolution.md
${OUT_DIR}/visual_reviews/<round_id>/*.json
${OUT_DIR}/protocol_diagnostics.json
```

`slidex_state.json`은 최소한 다음을 기록한다.

- state schema version과 tool version
- active deck id
- required Codex CLI version `0.132.0`
- selected runtime mode와 protocol bundle hash
- stage별 input/output path와 SHA-256
- stage별 status, verifier, stop condition
- goal objective, status, token budget, usage limit, repeated blocker signature
- unresolved risks와 accepted risks

`codex_threads.json`은 최소한 다음을 기록한다.

- Codex version
- thread id, thread name, stage
- model, service tier, approval policy, sandbox
- effective workspace roots
- token usage
- global/thread-scoped feature probe result
- output schema hash
- prompt template version

## 6. Pipeline 작업 흐름

Stage graph:

```text
resolve_workspace
  -> inspect_inputs
  -> intake
  -> strategy
  -> spec
  -> build_html
  -> baseline_html
  -> render
  -> qa
  -> revise_loop
  -> delivery_summary
  -> package
```

모든 stage는 동일한 contract를 따른다.

```json
{
  "stage": "build_html",
  "inputs": [
    {"path": "brief.md", "sha256": "..."},
    {"path": "out/deck_spec.json", "sha256": "..."}
  ],
  "outputs": [
    {"path": "out/final_deck.html", "sha256": "..."}
  ],
  "runtime": {
    "kind": "codex-app-server",
    "codexVersion": "0.132.0",
    "threadId": "...",
    "turnId": "...",
    "model": "...",
    "outputSchemaSha256": "..."
  },
  "verifier": {
    "name": "html_contract",
    "status": "pass"
  },
  "stopCondition": "pass"
}
```

Stop condition:

- `pass`
- `pass_with_risks`
- `blocked`
- `user_input_required`
- `usage_limited`
- `repeated_blocker`

Stage별 작업:

- `resolve_workspace`: `prompts/_active_deck_context.md` 규칙을 Go resolver로 구현한다.
- `inspect_inputs`: `brief.md`, `DESIGN.md`, assets, brand, data, source, reference docs를 inventory한다.
- `intake`: 입력 부족 시 한국어 질문을 `${OUT_DIR}/intake_questions.md`에 쓰고 exit code 3으로 멈춘다.
- `strategy`: audience, purpose, claims, evidence, risk policy를 문서화한다.
- `spec`: `schemas/deck_spec.schema.json`을 full JSON Schema validator로 검증한다.
- `build_html`: HTML writer를 단일 owner로 두고, reviewer는 patch suggestion만 낸다.
- `baseline_html`: current HTML을 `final_deck.generated_baseline.html`로 복사한다.
- `render`: Chrome DOM enumeration으로 `section.slide`를 enumerate하고 PNG/PDF/manifest를 생성한다.
- `qa`: deterministic findings와 Codex visual review findings를 병합한다.
- `revise_loop`: blocker/major finding을 보완하되 repeated blocker와 usage limit에서 자동 중단한다.
- `delivery_summary`: package보다 먼저 artifact list, QA status, accepted risks, review loop 결과를 요약한다.
- `package`: current HTML, render manifest, PNG set, visual review image set, QA report, delivery summary freshness를 마지막 gate로 검증한다.

## 7. Codex Runtime 계획

Runner interface:

```go
type Runner interface {
    StartStage(ctx context.Context, req StageRequest) (StageResult, error)
    ResumeStage(ctx context.Context, req ResumeRequest) (StageResult, error)
    Review(ctx context.Context, req ReviewRequest) (ReviewResult, error)
    Capabilities(ctx context.Context) (CapabilitySnapshot, error)
}
```

Backend:

- `app-server`: 기본 backend다. 장기 `slidex run`, goal tracking, visual QA, streamed event logging, approvals, review, MCP 상태 추적, remote TUI handoff에 사용한다.
- `exec`: `--ci`, `--codex-mode exec`, App Server unavailable fallback, short batch stage에 사용한다.
- `sdk-adapter`: future backend다. Go 단일 바이너리 원칙 때문에 초기 구현 대상은 아니다. 구현한다면 Python package는 `openai-codex`, import path는 `openai_codex`로 pin한다.

`--non-interactive`는 TUI를 열지 않는다는 뜻이지 App Server를 끈다는 뜻이 아니다. 기본은 App Server `stdio://`다.

## 8. App Server 작업 계획

Transport 우선순위:

1. `stdio://`: per-run child process로 실행하는 기본 local automation path
2. `unix://PATH`: 장기 공유 서버가 필요할 때 사용
3. `ws://127.0.0.1:PORT`: remote TUI handoff 또는 관찰 UI가 필요할 때만 사용
4. non-loopback WebSocket: 기본 금지, 허용 시 auth와 TLS/SSH tunnel 필수

Process lifecycle:

- `slidex codex app-server probe --listen stdio://`는 handshake 확인용으로 짧게 실행하고 종료한다.
- `start/status/stop`은 `unix://` 또는 localhost `ws://127.0.0.1:PORT` 장기 서버만 대상으로 한다.
- metadata path 예시는 `${XDG_RUNTIME_DIR}/slidex/codex-app-server.json`이다.
- `status`는 PID file, process alive, socket connect, `initialize` probe 순서로 확인한다.
- WebSocket 장기 서버에 한해서 `/readyz`, `/healthz`를 보조 확인한다.
- stale socket, orphan PID, 다른 user 소유 socket은 자동 재사용하지 않고 doctor finding으로 보고한다.

Protocol bundle:

```text
internal/codex/protocol/codex-cli-0.132.0/
  schema/
    ClientRequest.json
    ServerNotification.json
    ThreadStartParams.json
    TurnStartParams.json
    ...
  protocol_manifest.json
  generated_types.go
  method_constants.go
```

Schema refresh 절차:

1. `codex --version`이 `0.132.0`인지 확인한다.
2. `codex app-server generate-json-schema --help`를 실행한다.
3. `--experimental` 지원 여부를 확인하고, 지원될 때만 사용한다.
4. schema bundle을 생성한다.
5. `ClientRequest` method enum과 required params type을 확인한다.
6. `TurnStartParams.outputSchema` 존재 여부를 확인한다.
7. `UserInput.localImage`와 image fidelity 관련 field를 확인한다.
8. `thread/goal/*`, `review/start`, `thread/compact/start`, `experimentalFeature/list` optional `thread_id`를 optional method로 기록한다.
9. `PermissionProfile`을 required type으로 요구하지 않는다.
10. `protocol_manifest.json`, generated type, method constants를 작성한다.

Initialize와 turn 순서:

```text
connect
  -> initialize
  -> initialized notification
  -> model/list
  -> experimentalFeature/list
  -> mcpServerStatus/list
  -> thread/start
  -> experimentalFeature/list       # thread-scoped if optional thread_id exists
  -> optional thread/goal/set
  -> turn/start
```

`turn/start` output:

- App Server `outputSchema`로 final assistant message를 제한한다.
- 결과 JSON은 local full JSON Schema validator로 다시 검증한다.
- 검증 통과 전에는 파일 쓰기, patch apply, stage pass 처리를 하지 않는다.

Event logging:

- turn started/completed/failed
- item started/completed
- plan update
- diff update
- command execution summary
- MCP call start/complete/fail
- approval request/response
- review events
- subagent/thread fork evidence
- token usage
- service tier
- effective workspace roots

Absolute path는 기본 기록하지 않고 repo-relative path를 primary로 저장한다.

## 9. Exec fallback 계획

Fallback order:

```text
1. codex exec --json --output-schema <schema> --sandbox workspace-write
2. codex exec resume --last --json --output-schema <schema>
3. codex exec resume <SESSION_ID> --json --output-schema <schema>
4. codex exec --output-last-message <path> only for final text capture
5. interactive handoff: codex resume <thread_id> 또는 codex --remote ...
```

Fallback 산출물은 반드시 state에 기록한다.

```json
{
  "codexRuntime": {
    "mode": "exec_fallback",
    "reason": "app_server_initialize_failed",
    "missingCapabilities": ["thread/goal/set", "review/start"]
  }
}
```

CI auth policy:

- CI 기본은 API key auth다.
- `CODEX_API_KEY`는 `codex exec` 경로로만 처리한다.
- `~/.codex/auth.json`은 password-equivalent secret으로 취급한다.
- public/open-source runner에 ChatGPT-managed auth를 seed하지 않는다.
- logs에는 auth file path와 hash verifier만 남기고 raw token은 기록하지 않는다.

## 10. Rendering과 QA 계획

Rendering:

- Primary: Chrome runtime DOM enumeration
- Fallback: Go HTML parser
- Regex extraction은 smoke check나 legacy fallback에만 사용한다.
- Chrome `--no-sandbox`는 기본값에서 제거한다.
- root/container CI 또는 명시적 `--chrome-no-sandbox`에서만 사용하고, manifest와 QA report에 warning을 기록한다.

Render manifest 추가 필드:

```json
{
  "chromeSandbox": "enabled",
  "chromeNoSandboxReason": null,
  "slideEnumerationMethod": "chrome-dom",
  "repoRelativePaths": true
}
```

Visual QA:

- App Server mode에서는 rendered PNG를 `localImage` input으로 첨부한다.
- requested fidelity는 가능한 경우 `original`로 요청한다.
- reviewer output은 image hash, dimensions, slide id, fidelity를 포함해야 한다.
- image-capable model이 없으면 visual QA는 blocked finding으로 남긴다.
- exec fallback에서는 `codex exec --image <png> --output-schema schemas/review_findings.schema.json`을 사용한다.

QA report header 예시:

```yaml
slidexQaReport:
  schemaVersion: slidex.qaReport.v1
  generatedAt: 2026-05-20T...Z
  htmlSha256: ...
  renderManifestSha256: ...
  pngSetSha256: ...
  visualReviewImageSetSha256: ...
  deterministicStatus: pass
  visualStatus: pass
```

Deterministic fail은 Codex visual review 통과만으로 pass 처리할 수 없다.

## 11. Subagents, Parallel Review, Goal

두 실행 방식을 구분한다.

Codex subagent mode:

- parent turn prompt에서 명시적으로 subagent spawn을 요청한다.
- Codex가 orchestration한다.
- `slidex`는 spawned item/thread evidence, returned result, token usage를 추적한다.
- direct spawn App Server method가 generated schema에 없으면 protocol API처럼 가정하지 않는다.
- subagents는 parent sandbox/approval policy를 상속한다.

Parallel reviewer thread mode:

- `slidex`가 독립 App Server thread 여러 개를 만들고 reviewer prompt를 병렬 실행한다.
- artifact에는 `parallel_reviewer_threads`라고 기록한다.
- reviewer thread는 read-only 원칙을 따른다.
- HTML writer는 항상 단일 owner다.

Stage별 reviewer:

- Intake: source inventory reviewer, claim/evidence reviewer, design reference reviewer
- Strategy/spec: business structure reviewer, claim provenance reviewer, render feasibility reviewer
- Build: copy/design/accessibility reviewer
- QA/revise: visual/design/copy reviewer
- Finalize: delivery checklist reviewer, artifact freshness reviewer

Goal:

- `slidex run` 시작 시 검증 가능한 종료 조건을 goal로 설정한다.
- App Server goal API가 있으면 `thread/goal/*`를 사용한다.
- fallback mode에서는 local mirror만 기록한다.
- repeated blocker signature가 동일하게 반복되면 자동 revise loop를 멈춘다.
- usage limit, repeated blocker, user input required는 goal continuation을 지속하지 않는다.

## 12. Skills, Plugins, Hooks, MCP

Companion skill 경로:

```text
.agents/skills/slidex/SKILL.md
.agents/skills/slidex/references/commands.md
.agents/skills/slidex/scripts/slidex-doctor.sh
```

금지된 경로:

```text
.codex/skills/slidex/SKILL.md
```

Plugin:

- optional distribution layer다.
- canonical interface는 `slidex` CLI다.
- plugin은 Codex UI discoverability, bundled skill, optional MCP config, read-only hooks를 제공한다.
- plugin marketplace/share checkout은 production dependency가 아니다.

Hooks:

- 허용: HTML 직접 수정 감지 후 `slidex qa` 권고, dependency 변경 감지 후 `slidex render` 권고, stale manifest 감지 후 `slidex package` fail 안내
- 금지: hook이 자동으로 HTML 수정, Codex turn 시작, approval 없이 shell command 실행

MCP:

- `slidex codex doctor`는 `codex mcp list`와 App Server MCP status를 확인한다.
- `.codex/config.toml`은 trusted project에서 MCP 설정용으로 허용한다.
- OpenAI docs MCP는 최신 기능 확인에 유용하지만 deck production의 필수 runtime dependency는 아니다.
- `slidex mcp-server --stdio`는 동일한 Go deterministic 구현을 호출해야 하며 별도 코드 경로를 만들지 않는다.

## 13. Config 계획

```toml
# slidex.toml
[workspace]
default_deck = "spikalabs"

[codex]
mode = "app-server"
required_version = "0.132.0"
protocol_bundle = "codex-cli-0.132.0"
model_strategy = "catalog-default"
analysis_model = "catalog-default"
analysis_reasoning_effort = "xhigh"
development_model = "catalog-default"
development_reasoning_effort = "high"
sandbox = "workspace-write"
approval_policy = "on-request"
enable_goals = true
enable_subagents = true
enable_thread_scoped_feature_probe = true
allow_protocol_mismatch = false

[codex.exec]
allow_fallback = true
use_resume_output_schema = true
ci_auth_mode = "api-key"

[codex.app_server]
default_transport = "stdio://"
shared_transport = "unix://"
websocket_allowed = false
websocket_auth_required = true

[render]
width = 1920
height = 1080
font_preset = "pretendard"
pdf_mode = "paginated"
chrome_no_sandbox = false
slide_enumeration = "chrome-dom"

[qa]
visual_review = "codex"
image_fidelity = "original"
max_revise_rounds = 3
repeated_blocker_threshold = 2

[logs]
secure_file_mode = "0600"
secure_dir_mode = "0700"
verbose_log = false
retention_successful_runs = 1
retention_failed_runs = 1
```

Config 원칙:

- model catalog는 App Server `model/list`를 우선한다.
- model override는 사용자가 강제한 경우에만 적용한다.
- exact pin 정책 때문에 `required_version`은 range가 아니라 정확한 버전 문자열이다.
- `doctor`는 `.mise.toml`, `go.mod`, Chrome version, Codex CLI version, App Server schema, feature flags, MCP status, plugin/skill path를 검사한다.

## 14. 보안과 격리

App Server/WebSocket:

- WebSocket은 localhost 외부 노출 금지다.
- non-loopback bind 시 auth와 TLS/SSH tunneling이 필수다.
- token file은 `0600`이어야 한다.
- raw bearer token은 command line에 넣지 않는다.

Filesystem:

- generated artifact는 `${OUT_DIR}` 아래에만 쓴다.
- user-supplied source 문서는 deck workspace 밖으로 복사하지 않는다.
- state/log/cache는 `.gitignore` 또는 deck output ignore policy에 명시한다.

State/log security:

- 대상: `slidex_state.json`, `codex_threads.json`, `run_log.jsonl`, `agent_runs/`, `agent_reviews/`, `visual_reviews/`, `protocol_diagnostics.json`
- 파일 권한은 `0600`, 디렉터리는 `0700`이다.
- token, bearer credential, cookie, API key, auth header, capability token 원문은 기록하지 않는다.
- redaction pattern은 `OPENAI_API_KEY`, `CODEX_API_KEY`, `Authorization`, `Bearer`, `*_TOKEN`, `*_SECRET`, `cookie`, `set-cookie`를 포함한다.
- command output logging은 기본 summary mode다.
- full event logging은 `--verbose-log` opt-in일 때만 켠다.
- `slidex package --include-logs`는 sanitizer를 통과한 log만 포함한다.
- retention 기본값은 최신 successful run 1개와 직전 failed run 1개다.

Chrome sandbox:

- 기본은 sandbox enabled다.
- `--chrome-no-sandbox`는 명시적 flag가 있어야 한다.
- 사용 시 warning과 risk를 render manifest, QA report, state에 남긴다.

## 15. Exit codes와 출력

```text
0 pass
1 fail finding 또는 command failure
2 usage/config error
3 user input required
4 blocked by unsupported Codex/App Server feature
5 stale artifacts
6 accepted risks exist, but requested gate was strict pass
7 usage limit reached
8 repeated blocker reached
```

출력 contract:

- 기본 stdout은 최종 artifact summary다.
- progress는 stderr로 보낸다.
- `--json`은 machine-readable JSON을 출력한다.
- 장기 명령은 JSONL event stream option을 제공한다.

## 16. 구현 Phase

### Phase 0: Fixture와 regression harness

목표: Codex/App Server 구현 전에 deterministic baseline을 고정한다.

작업:

- `fixtures/minimal_deck` 추가
- `fixtures/spikalabs_snapshot` 추가
- render/qa/package golden output 작성
- no-Codex deterministic e2e 작성
- `go test ./...` harness 작성
- current command stdout/stderr/exit code snapshot 작성

Acceptance:

- fixture HTML 1개로 render -> qa -> package deterministic path가 통과한다.
- 기존 deterministic 명령의 기본 동작이 깨지지 않는다.
- stale artifact를 의도적으로 만들면 package가 exit code 5로 실패한다.

### Phase 1: CLI 구조화와 internal package 분리

작업:

- `cmd/slidex/main.go`를 thin entrypoint로 축소한다.
- `internal/workspace`, `internal/render`, `internal/qa`, `internal/packager`, `internal/schema`를 만든다.
- `--deck` resolver를 공통화한다.
- secure writer를 도입한다.

Acceptance:

- 기존 명령은 계속 동작한다.
- `render`, `qa`, `package`가 internal package를 호출한다.
- `--deck` 기반 path resolution이 render/qa/sync/package에 적용된다.

### Phase 2: State, config, lock, secure logging

작업:

- `slidex.toml`을 도입한다.
- `slidex_state.json`, `codex_threads.json`, `run_log.jsonl` writer를 만든다.
- concurrent lock을 추가한다.
- redaction/sanitizer를 추가한다.
- retention policy를 구현한다.

Acceptance:

- state/log 파일은 `0600`, directory는 `0700`이다.
- 동시 `slidex run`은 lock으로 차단된다.
- input/output hash가 stage별로 기록된다.
- stale HTML 수정 후 render/qa/package 중 적절한 gate에서 막힌다.

### Phase 3: Schema와 prompt template 내장

작업:

- full JSON Schema validator를 도입한다.
- prompt templates를 vendoring한다.
- `intake`, `strategy`, `spec`, `build`, `revise`, `finalize` 명령을 추가한다.
- stage output schemas를 추가한다.
- mock runner를 추가한다.

Acceptance:

- prompt template rendering snapshot test가 통과한다.
- invalid agent output은 file write 전에 fail한다.
- mock runner로 strategy/spec/build stage가 재현된다.

### Phase 4: Codex exec runner

작업:

- `codex exec --json` runner를 구현한다.
- `codex exec --output-schema`를 지원한다.
- `codex exec resume --output-schema`를 지원한다.
- `--output-last-message`는 보조 capture로만 지원한다.
- CI auth policy를 구현한다.

Acceptance:

- fresh exec와 resume exec 모두 schema output을 생성한다.
- fallback risk가 state와 QA report에 기록된다.
- `CODEX_API_KEY` redaction test가 통과한다.

### Phase 5: App Server probe/client

작업:

- process manager를 구현한다.
- stdio JSONL client를 구현한다.
- `initialize`와 `initialized`를 구현한다.
- `model/list`, `experimentalFeature/list`, `mcpServerStatus/list`를 probe한다.
- schema refresh와 protocol compatibility gate를 구현한다.

Acceptance:

- generated schema의 required method가 없으면 doctor exit code 4다.
- 0.132.0 protocol bundle이 설치 Codex version과 일치한다.
- optional `thread_id` feature probe가 있으면 thread-scoped probe를 수행한다.
- `PermissionProfile` 부재를 failure로 처리하지 않는다.

### Phase 6: App Server turn runner와 full run orchestrator

작업:

- `thread/start`를 구현한다.
- `turn/start + outputSchema`를 구현한다.
- event logging을 구현한다.
- `thread/read` 또는 `thread/turns/list` resume path를 구현한다.
- pipeline graph execution, skip/resume/retry를 구현한다.

Acceptance:

- `slidex run --deck <fixture> --until package --codex-mode app-server` e2e가 통과한다.
- streamed events가 state와 run_log에 기록된다.
- App Server forced failure 시 exec fallback이 동작한다.

### Phase 7: Goal, visual QA, review loop

작업:

- `slidex goal` wrapper를 구현한다.
- goal mirror를 구현한다.
- usage limit/repeated blocker stop을 구현한다.
- visual QA localImage와 original fidelity를 구현한다.
- structured review turn을 구현한다.
- parallel reviewer thread mode를 구현한다.
- Codex subagent mode prompt support를 구현한다.

Acceptance:

- goal objective validation이 통과한다.
- usage limit 또는 repeated blocker에서 자동 loop가 멈춘다.
- 최소 1장 PNG가 localImage로 전달되고 image hash/dimensions/fidelity가 output에 기록된다.
- reviewer blocker/major issue가 해결되거나 accepted risk로 남는다.

### Phase 8: MCP server, skills, plugin, hooks, remote-control

작업:

- `.agents/skills/slidex/SKILL.md`를 추가한다.
- optional plugin package를 설계한다.
- read-only advisory hooks를 구현한다.
- `slidex mcp-server --stdio`를 구현한다.
- remote-control status integration을 추가한다.

Acceptance:

- Codex가 repository skill을 발견한다.
- plugin hooks는 read-only advisory만 수행한다.
- MCP server가 `inspect`, `render`, `qa`, `package`, `state/read`를 동일 Go 구현으로 호출한다.

## 17. 최종 Acceptance Criteria

완료 조건:

1. `slidex run --deck <deck>`만으로 intake gate부터 delivery summary와 package까지 실행할 수 있다.
2. 자료가 부족하면 한국어 질문을 남기고 exit code 3으로 멈춘다.
3. 사용자는 prompt 파일을 직접 실행하지 않아도 모든 표준 stage를 CLI로 실행할 수 있다.
4. 기존 deterministic 명령은 계속 동작한다.
5. `slidex doctor --codex`가 Codex CLI 0.132.0, App Server protocol schema, feature flags, goal availability, MCP status, plugin/skill path, `codex doctor` 결과를 검사한다.
6. App Server mode는 initialize, initialized, capability snapshot, model/list, experimentalFeature/list global/thread-scoped, mcpServerStatus/list를 수행한다.
7. App Server mode에서 thread/start, turn/start, outputSchema, streamed event logging, thread/read 또는 thread/turns/list, structured review, optional review/start, thread/goal/*가 stage state에 기록된다.
8. `PermissionProfile`이 protocol bundle에 없어도 실패하지 않는다.
9. `codex exec resume --output-schema` fallback/resume path가 동작한다.
10. `/goal`과 `slidex goal`의 차이가 help와 docs에 명확하다.
11. goal objective non-empty, 4,000자 이하 validation이 있다.
12. goal continuation은 usage limit 또는 repeated blocker에서 멈춘다.
13. visual QA는 App Server localImage 또는 fallback `codex exec --image`로 rendered PNG를 전달한다.
14. visual QA integration test가 original image fidelity, image hash, dimensions를 검증한다.
15. structured reviewer loop가 design, HTML, QA/delivery gate에서 실행된다.
16. Codex subagent mode와 parallel reviewer thread mode가 artifact에서 구분된다.
17. deterministic QA fail은 Codex review 통과만으로 pass 처리되지 않는다.
18. `package`는 QA report freshness, visual review image set freshness, delivery summary freshness를 모두 검증한다.
19. Chrome `--no-sandbox`는 기본 사용되지 않는다. 사용 시 warning/risk가 기록된다.
20. state/log 파일 권한, redaction, sanitizer, retention이 테스트된다.
21. `slidex package --include-logs`는 sanitizer를 통과한 log만 포함한다.
22. `slidex clean --logs --older-than <duration>`은 retention 정책에 따라 로그를 정리한다.
23. stale artifact detection, resume/retry, concurrent run lock, app-server PID/socket lifecycle, WebSocket keepalive/retry가 테스트된다.
24. `.agents/skills/slidex/SKILL.md` companion skill이 발견된다.
25. PPTX는 생성/선택 납품물로 다시 등장하지 않는다.
26. `mise exec -- go test ./...`가 통과한다.

## 18. 열린 결정 사항

1. App Server client를 Go에서 직접 구현할지, Python `openai-codex` SDK adapter를 child process로 둘지 결정해야 한다. 초기 구현은 Go direct client가 목표다.
2. model selection은 App Server `model/list` catalog default를 우선하되, 프로젝트가 요구하는 reasoning effort와 맞지 않을 때의 fallback policy가 필요하다.
3. Chrome DOM enumeration을 primary로 구현할지, Go HTML parser를 primary로 구현할지 선택해야 한다. 권장은 Chrome DOM enumeration primary, Go parser fallback이다.
4. companion plugin을 repository에 포함할지 별도 배포물로 관리할지 결정해야 한다.
5. WebSocket remote-control을 어느 수준까지 `slidex`에서 직접 관리할지 결정해야 한다. 기본 production path는 stdio App Server이므로 remote-control은 advanced path로 유지한다.

## 19. 실행 순서 요약

첫 구현 단위는 Phase 0이다. App Server나 Codex runner부터 붙이면 protocol drift와 agent variability가 먼저 들어오므로, fixture와 deterministic regression harness를 먼저 고정한다.

권장 진행 순서:

1. Phase 0으로 fixture와 기존 명령 회귀 테스트를 만든다.
2. Phase 1로 CLI entrypoint와 internal package 경계를 정리한다.
3. Phase 2로 state/config/security foundation을 만든다.
4. Phase 3으로 schema validator와 prompt template을 내장한다.
5. Phase 4로 `codex exec` runner를 먼저 구현해 fallback 자동화를 확보한다.
6. Phase 5와 Phase 6에서 App Server protocol bundle, probe, turn runner, orchestrator를 붙인다.
7. Phase 7에서 goal, visual QA, structured review, parallel reviewer를 붙인다.
8. Phase 8에서 MCP server, companion skill/plugin/hooks, remote-control을 보조 배포 계층으로 추가한다.
