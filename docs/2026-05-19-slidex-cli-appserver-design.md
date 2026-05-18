# 2026-05-19 slidex CLI/App Server 고도화 설계

Status: Final design after three GPT-5.5 Xhigh review rounds on 2026-05-19.

이 문서는 2026-05-18 HTML/PDF 전환 설계를 잇는 후속 고도화 설계다.
목표는 `slidex`를 "Codex가 프롬프트를 실행할 때 보조로 쓰는 렌더링 CLI"가
아니라, 문서 생성 전 과정을 소유하는 완전한 로컬 CLI 프로그램으로 승격하는
것이다.

## 목표

1. `slidex`를 완전 CLI 프로그램으로 만든다.
   - 사용자는 `codex exec - < prompts/...`를 직접 조합하지 않고
     `slidex run`, `slidex intake`, `slidex build`, `slidex qa` 같은 명령으로
     전체 워크플로를 실행한다.
   - 기존 prompt 파일은 폐기하지 않고 CLI에 내장되는 durable prompt template로
     재사용한다.
   - 렌더링/검증처럼 결정적인 작업은 Go 코드가 직접 수행하고, 전략/작성/리뷰처럼
     에이전트 판단이 필요한 작업은 Codex 실행 계층이 수행한다.

2. 내부적으로 Codex App Server를 최대한 활용한다.
   - `codex app-server`를 로컬 제어 평면으로 사용해 thread, turn, approval,
     streamed event, model catalog, feature catalog, MCP 상태, review, goal 상태를
     추적한다.
   - App Server 프로토콜은 Codex CLI 버전에 묶인 실험적 계약이므로, 생성된
     JSON Schema를 버전별로 보관하고 런타임에서 호환성을 확인한다.
   - 단, 공식 문서상 단순 CI/배치 자동화는 `codex exec` 또는 SDK가 더 적합할 수
     있으므로, "최대한 활용"은 모든 작업을 App Server로 강제한다는 뜻이 아니다.
     장기 세션, goal, streamed event, approval, review, remote handoff가 필요한
     곳에서는 App Server를 우선하고, 짧은 batch stage에서는 `codex exec --json`을
     안정 경로로 허용한다.

3. 2026년 5월 기준 Codex CLI 기능을 최대한 활용한다.
   - 확인된 로컬 기준: `codex-cli 0.130.0`.
   - `features.goals`는 experimental이지만 현재 활성화되어 있었고, `/goal`은
     독립 shell subcommand가 아니라 TUI slash command다.
   - App Server에는 같은 goal 상태를 다루는 `thread/goal/set`,
     `thread/goal/get`, `thread/goal/clear` 실험 API가 있다.
   - non-interactive mode, JSONL events, output schema, review, subagents,
     MCP, skills, plugins, hooks, remote TUI, feature flags를 CLI 설계에 반영한다.

## 현재 상태 요약

- 현재 Go 진입점은 단일 파일 `cmd/slidex/main.go`이며 명령은
  `inspect`, `validate-spec`, `render`, `qa`, `sync-html-edits`, `package`로
  제한되어 있다. 근거: `cmd/slidex/main.go:147`.
- 기존 2026-05-18 설계도 동일한 최소 명령 세트를 제안하고, 렌더/QA/패키징을
  중심으로 acceptance를 정의했다. 근거: `docs/2026-05-18-update-design.md:929`.
- 현재 `one_shot_create_business_doc.md`는 전체 흐름을 여전히 Codex prompt 실행
  절차로 정의한다. 근거: `prompts/one_shot_create_business_doc.md:19`.
- `AGENTS.md`는 완전한 HTML/PDF 산출물 계약과 QA 게이트를 이미 정의한다.
  근거: `AGENTS.md:91`.
- schema는 HTML/PDF 중심이며 `renderConfig.engine = slidex-cli`를 요구한다.
  근거: `schemas/deck_spec.schema.json:101`.

결론: 현재 `slidex`는 렌더링/검증 자동화 CLI로는 출발했지만, intake,
strategy, spec, HTML authoring, revision, Codex thread orchestration,
goal/subagent/review 관리는 아직 CLI가 소유하지 않는다. 또한 `package`는 현재
`delivery_summary.md` 존재를 요구하지만 finalize prompt는 package 이후
delivery summary를 쓰도록 안내하는 순서 충돌이 있어, 완전 CLI 전환 시 stage
순서를 바로잡아야 한다.

## 공식/로컬 근거

확인일: 2026-05-19 KST.

- OpenAI Codex CLI 문서는 Codex CLI가 로컬 터미널에서 저장소를 읽고, 수정하고,
  명령을 실행할 수 있는 coding agent라고 설명한다.
  Source: https://developers.openai.com/codex/cli
- Codex CLI features 문서는 `codex app-server --listen ws://127.0.0.1:4500`와
  `codex --remote ws://127.0.0.1:4500`를 통한 remote TUI 연결을 설명한다.
  Source: https://developers.openai.com/codex/cli/features
- command line reference는 `--cd`, `--sandbox`, `--ask-for-approval`, `--model`,
  `--image`, `--search`, `--remote`, `--enable`, `--disable`, `--profile` 등을
  전역 실행 제어 축으로 문서화한다.
  Source: https://developers.openai.com/codex/cli/reference
- slash command 문서는 `/goal`이 `features.goals`가 켜져 있을 때 사용할 수
  있는 experimental command이며, objective를 설정/조회/pause/resume/clear할 수
  있다고 설명한다.
  Source: https://developers.openai.com/codex/cli/slash-commands
- App Server 문서는 JSON-RPC 2.0 스타일 protocol, stdio/websocket/unix socket
  transport, schema generation, thread/turn/review/goal API를 제공한다고 설명한다.
  Source: https://developers.openai.com/codex/app-server
- non-interactive mode 문서는 `codex exec`, JSONL event stream, `--output-schema`,
  sandbox/approval 설정을 자동화 경로로 문서화한다.
  Source: https://developers.openai.com/codex/noninteractive
- Codex SDK 문서는 CI/CD나 내부 도구 통합에는 SDK가 App Server보다 적절할 수
  있음을 시사한다. 따라서 `slidex`는 로컬 rich integration에는 App Server를
  우선하되, CI fallback은 `codex exec --json` 또는 SDK adapter가 가능하게 둔다.
  Source: https://developers.openai.com/codex/sdk
- subagents 문서는 복잡한 작업에서 전문 agent를 병렬로 띄워 결과를 모을 수
  있고, 현재 Codex app과 CLI에서 surfaced된다고 설명한다.
  Source: https://developers.openai.com/codex/subagents
- MCP 문서는 Codex CLI와 IDE extension이 MCP 설정을 공유하며, CLI에서
  `codex mcp`로 서버를 관리할 수 있다고 설명한다.
  Source: https://developers.openai.com/codex/mcp
- "Create a CLI Codex can use" use case는 Codex가 반복 작업에 활용할 수 있는
  composable CLI를 만들고 PATH 설치, auth/setup check, JSON output을 검증하는
  방향을 제안한다.
  Source: https://developers.openai.com/codex/use-cases/agent-friendly-clis
- "Follow a goal" use case는 `/goal`을 장기 작업의 검증 가능한 종료 조건과
  함께 쓰라고 설명한다.
  Source: https://developers.openai.com/codex/use-cases/follow-goals

## 설계 원칙

- `slidex`가 사용자-facing workflow owner다. Codex CLI는 내부 agent runtime이다.
- Codex 실행 모드는 stage 성격에 맞춘다. rich session은 App Server, batch
  automation은 `codex exec --json`, future server-side integration은 SDK adapter를
  선택할 수 있어야 한다.
- deterministic engine과 generative agent를 분리한다.
  - Go: workspace resolution, file inventory, schema validation, HTML parse,
    render, PDF packaging, manifest freshness, image blank detection, QA findings,
    state persistence, exit code.
  - Codex: intake 질문 생성, 전략/문안/HTML 작성, claim rewrite, reviewer loop,
    design critique, visual QA reasoning.
- 모든 단계는 재시작 가능해야 한다. CLI가 thread id, goal, input hash, output
  hash, stage status를 기록한다.
- 모든 agent 산출물은 파일로 남긴다. 최종 보고는 transcript 기억이 아니라
  `${OUT_DIR}` artifacts에 기반한다.
- PPTX 생성은 여전히 금지한다. 사용자 PPTX는 source/reference만 허용한다.
- App Server는 로컬 제어 평면으로만 사용한다. `slidex`는 hosted service나 SaaS가
  아니다.

## 목표 아키텍처

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
internal/config/
  config.go
  codex.go
internal/prompts/
  embed.go
  templates/*.md
internal/codex/
  runner.go
  exec_runner.go
  appserver_client.go
  appserver_process.go
  protocol/codex-cli-0.130.0/*.json
  events.go
  goals.go
  reviews.go
  subagents.go
internal/pipeline/
  graph.go
  intake.go
  strategy.go
  spec.go
  build.go
  revise.go
  finalize.go
internal/render/
internal/pdf/
internal/montage/
internal/qa/
internal/sync/
internal/schema/
internal/report/
```

### 핵심 데이터 파일

```text
${OUT_DIR}/slidex_state.json
${OUT_DIR}/codex_threads.json
${OUT_DIR}/run_log.jsonl
${OUT_DIR}/agent_reviews/round_01/reviewer_*.md
${OUT_DIR}/agent_reviews/round_01/resolution.md
${OUT_DIR}/agent_runs/<stage>/<thread_id>.json
```

`slidex_state.json`은 stage별 status, input/output hashes, active Codex thread,
goal status, unresolved risks, last verified command를 기록한다.

`codex_threads.json`은 App Server thread id, session id, forked thread id,
model, reasoning effort, approval policy, sandbox, serviceName, prompt template
version, output artifact path를 기록한다.

## CLI 명령 설계

### 기본 명령

```bash
slidex init <deck_id> [--from-template decks/_template]
slidex doctor [--deck decks/<deck_id>] [--codex] [--render]
slidex inspect --deck decks/<deck_id> [--write]
slidex intake --deck decks/<deck_id> [--interactive|--answers FILE]
slidex strategy --deck decks/<deck_id>
slidex spec --deck decks/<deck_id>
slidex build --deck decks/<deck_id>
slidex render --deck decks/<deck_id>
slidex qa --deck decks/<deck_id> [--visual-review codex|manual|none]
slidex revise --deck decks/<deck_id> [--until pass|risk-accepted]
slidex sync-html-edits --deck decks/<deck_id>
slidex finalize --deck decks/<deck_id>
slidex package --deck decks/<deck_id> [--include-logs]
slidex clean --deck decks/<deck_id> [--logs] [--older-than DURATION]
slidex run --deck decks/<deck_id> [--until package|qa|render] [--non-interactive]
```

기존 `render --html ... --pdf ...` 형태는 고급 override로 유지하되, 일반 사용자는
`--deck` 중심으로 실행한다. `slidex run`은 전체 directed acyclic pipeline을
실행하고, 이미 완료된 stage는 input hash가 바뀌지 않았다면 skip할 수 있다.

### Codex 통합 명령

```bash
slidex codex doctor
slidex codex app-server start [--listen unix://|ws://127.0.0.1:PORT]
slidex codex app-server status
slidex codex app-server stop
slidex codex app-server probe [--listen stdio://]
slidex codex schema refresh
slidex codex models
slidex codex features
slidex codex mcp
slidex codex threads list [--deck decks/<deck_id>]
slidex codex threads read <thread_id>
slidex codex review --deck decks/<deck_id> [--stage design|html|qa|delivery]
slidex mcp-server [--stdio]
slidex migrate --deck decks/<deck_id> [--from legacy-html-pdf|pptx-first]
```

### Goal 명령

```bash
slidex goal set --deck decks/<deck_id> --objective "..." [--token-budget N]
slidex goal status --deck decks/<deck_id>
slidex goal pause --deck decks/<deck_id>
slidex goal resume --deck decks/<deck_id>
slidex goal complete --deck decks/<deck_id>
slidex goal clear --deck decks/<deck_id>
```

구현 규칙:

- App Server가 사용 가능하고 `features.goals` 및 protocol capability가 확인되면
  `thread/goal/set`, `thread/goal/get`, `thread/goal/clear`를 사용한다.
- 지원 status enum은 vendored App Server JSON Schema에서 읽는다. 로컬
  `codex-cli 0.130.0`에서 생성한 experimental schema 기준
  `ThreadGoalStatus = active | paused | budgetLimited | complete`가 확인되었다.
  구현은 이 enum을 schema generation 결과에서 가져오고 문서 상수로만 복제하지
  않는다.
- App Server goal을 설정하면 같은 objective를 `slidex_state.json`에도 mirror로
  기록한다.
- fallback 모드에서는 `slidex_state.json`에만 기록하고, 사용자에게 interactive
  CLI에서 `/goal <objective>`를 쓰면 같은 목표가 Codex thread에 붙는다고 안내한다.

## Codex 실행 모드 선택

`slidex`는 하나의 Codex runner interface를 두고 세 가지 backend를 지원한다.

1. `app-server`
   - 기본값: 로컬 interactive run, 장기 `slidex run`, reviewer loop, goal 추적,
     remote TUI handoff.
   - 장점: thread history, streamed events, approvals, review endpoint,
     `thread/goal/*`, model/feature/MCP catalog를 직접 제어한다.
   - 단점: experimental protocol이며 버전 schema 검증이 필요하다.
2. `exec`
   - 사용 조건: `--ci`, `--codex-mode exec`, App Server unavailable fallback,
     또는 사용자가 명시한 short batch stage.
   - 장점: `codex exec --json --output-schema`로 안정적인 자동화가 쉽고 shell
     integration이 단순하다.
   - 단점: App Server goal/review/thread UI와 같은 rich session 기능이 약하다.
3. `sdk-adapter`
   - 후보: 장기적으로 TypeScript Codex SDK를 child process adapter로 두는 방식.
   - 단일 Go 바이너리 배포 목표와 충돌할 수 있으므로 Phase 4에서 재평가한다.

실행 모드는 `slidex.toml`의 `[codex].mode`와 command flag `--codex-mode`로
선택한다. 기본 정책은 local run, headless run, `--non-interactive` 모두
App Server `stdio://`다. `--non-interactive`는 "TUI를 열지 않는다"는 의미이지
App Server를 쓰지 않는다는 의미가 아니다. `exec`는 `--ci`, 명시적
`--codex-mode exec`, App Server 초기화 실패, 또는 사용자가 선택한 fallback일 때만
쓴다.

## App Server 사용 방식

### Transport

우선순위:

1. `stdio://`: `slidex`가 child process로 `codex app-server --listen stdio://`를
   실행하고 JSONL stream을 직접 제어한다. 기본 로컬 자동화 경로다.
2. `unix://PATH`: 장기 실행 app-server를 공유해야 할 때 사용한다.
3. `ws://127.0.0.1:PORT`: remote TUI handoff나 관찰 UI가 필요할 때만 사용한다.
4. non-loopback websocket은 기본 금지다. 반드시 `--ws-auth capability-token`
   또는 `--ws-auth signed-bearer-token`과 TLS/터널링 정책을 요구한다.

### Process Lifecycle

`slidex codex app-server start/status/stop`을 제공하려면 장기 공유 서버와
per-run child process lifecycle을 명확히 분리해야 한다.

- `stdio://`는 `slidex run` 내부의 per-run child process transport다. `start`,
  `status`, `stop`의 장기 공유 서버 대상이 아니다.
- `slidex codex app-server probe --listen stdio://`는 schema/handshake 확인을 위해
  짧게 child process를 띄우고 종료하는 진단 명령이다.
- 장기 공유 서버는 `unix://` 또는 localhost `ws://127.0.0.1:PORT`만 지원한다.
- app-server process는 user-scoped runtime directory 아래 PID file과 socket metadata를
  남긴다. 예: `${XDG_RUNTIME_DIR}/slidex/codex-app-server.json`.
- metadata에는 PID, start time, Codex version, listen URL, socket path, owner UID,
  auth mode, token file path 또는 token hash, attached deck ids를 기록한다.
- `status`는 PID 생존 여부, socket 연결 가능 여부, `/readyz` 또는 initialize probe
  성공 여부를 함께 확인한다.
- `stop`은 먼저 graceful termination을 시도하고, timeout 후 `--force`가 있을 때만
  강제 종료한다.
- orphan PID, stale socket, 다른 user가 소유한 socket은 자동으로 재사용하지 않고
  `doctor` finding으로 보고한다.

### Protocol 계약

- `slidex codex schema refresh`는 다음 명령을 실행해 현재 Codex CLI 버전의
  protocol schema를 vendoring한다.

```bash
codex app-server generate-json-schema --experimental --out internal/codex/protocol/codex-cli-<version>
```

- schema bundle에는 `ThreadGoalSetParams`, `ThreadStartParams`, `TurnStartParams`,
  `ReviewStartParams`, `ModelListParams`, `ExperimentalFeatureListParams`,
  `ListMcpServerStatusParams`, `CommandExecParams` 같은 타입이 존재해야 한다.
  실제 request method 이름은 generated `ClientRequest.json`의 enum을 source of
  truth로 삼는다. 로컬 `codex-cli 0.130.0` 기준 MCP status method는
  `mcpServerStatus/list`다.
- 런타임 시작 시 `codex --version`과 vendored protocol version을 비교한다.
  patch-level mismatch는 warning, minor/major mismatch는 `slidex codex doctor`에서
  fail로 처리한다. 사용자가 `--allow-codex-protocol-mismatch`를 명시해야 진행한다.

### Thread/Turn 흐름

0. `initialize` and `initialized`
   - transport 연결 직후 `initialize` request를 먼저 보낸다. 그 전의 모든 request는
     거부되어야 한다.
   - `clientInfo.name = "slidex"`, `title = "slidex CLI"`, current CLI version을
     보낸다.
   - goal, background terminal cleanup, process APIs, dynamic tools 같은 실험 API가
     필요한 run에서는 `capabilities.experimentalApi = true`를 명시한다.
   - server response에서 user agent, platform, capability 관련 값을
     `codex_threads.json`과 `run_log.jsonl`에 snapshot으로 기록한다.
   - 이어서 `initialized` notification을 보낸다.
   - 그 다음 `model/list`, `experimentalFeature/list`, `mcpServerStatus/list`를
     호출해 model catalog, feature state, MCP readiness를 기록한다.
   - initialize나 capability probe가 실패하면 `slidex`는 fallback 가능 여부를
     판단하고, 실패 원인을 structured finding으로 기록한다.
1. `thread/start`
   - `cwd`: repository root
   - `approvalPolicy`: stage별 기본값. production run은 `never` 또는 `on-request`
     중 사용자가 선택한다.
   - `sandbox`: thread-level `ThreadStartParams.sandbox`에는 generated schema의
     `SandboxMode` 값을 그대로 사용한다. 로컬 `codex-cli 0.130.0` 기준 기본값은
     `workspace-write`다. turn-level `sandboxPolicy` 객체의 값과 혼동하지 않는다.
   - `serviceName`: `slidex`
2. `thread/goal/set`
   - 장기 `slidex run`에는 검증 가능한 종료 조건을 goal로 설정한다.
3. `turn/start`
   - stage prompt, file mentions, `outputSchema`, previous artifact hashes를
     입력한다. 로컬 `codex-cli 0.130.0` App Server schema의 `TurnStartParams`에는
     final assistant message를 제한하는 optional `outputSchema` field가 확인되었다.
4. streamed events
   - `turn/started`, `item/*`, `turn/plan`, `turn/diff`, command executions,
     MCP calls, review events를 `run_log.jsonl`에 저장한다.
5. `thread/read` 또는 `thread/turns/list`
   - 재시작 시 이전 stage 근거와 final response를 복원한다.
6. `thread/compact/start`
   - 긴 문서 생산 중 context가 커지면 stage boundary에서만 수행하고, compact
     전후 summary를 artifact로 저장한다.
7. structured review turn
   - design/html/qa/delivery review는 기본적으로 `turn/start + outputSchema`로
     실행한다. reviewer findings schema를 강제해야 하기 때문이다.
   - `review/start`는 Codex native code-review UX가 필요할 때의 보조 경로로만
     사용하고, structured gate의 단독 근거로 삼지 않는다.
8. `turn/interrupt`, `turn/steer`
   - 사용자 개입이나 실패 복구에 사용한다.

### App Server와 `codex exec` fallback

App Server가 실패하면 다음 순서로 낮춘다.

1. `codex exec --json --output-schema <schema> --sandbox workspace-write`
2. `codex exec --output-last-message <path>`
3. interactive handoff: 사용자가 `codex resume <thread_id>` 또는 `codex --remote`로
   이어받는다.

fallback으로 생성된 산출물은 반드시 `slidex_state.json`에
`codexRuntime.mode = exec_fallback`으로 기록한다. goal, streamed approval,
thread history UI, review endpoint 등 일부 기능이 빠진다는 risk도 기록한다.
반대로 사용자가 `--codex-mode exec`를 명시했다면 이는 fallback이 아니라 선택된
runtime이므로 risk severity를 낮추되, App Server 전용 기능은 unavailable로 표시한다.

## Codex CLI 2026년 5월 기능 활용 계획

### `/goal`

- `slidex run` 시작 시 objective와 stopping condition을 하나의 goal로 설정한다.
- `slidex goal status`는 App Server goal usage와 local pipeline progress를 함께
  표시한다.
- goal 완료는 "현재 HTML에서 렌더된 PNG/PDF가 존재하고 QA/패키징/검토 로그가
  모두 현재 input hash와 일치"할 때만 허용한다.

### Subagents

`slidex`는 병렬화 가능한 agent 작업을 stage별로 나눈다. 여기서 두 실행 방식을
구분한다.

- Codex subagent mode: parent `turn/start` prompt에서 Codex에게 명시적으로
  subagent를 spawn하도록 요청한다. App Server client는 spawned agent item,
  thread/fork 관련 notification, final returned findings를 추적한다. 직접적인
  "spawn subagent" App Server method가 문서화되어 있지 않다면 이를 protocol API처럼
  가정하지 않는다.
- Parallel reviewer thread mode: `slidex`가 독립 App Server thread 여러 개를
  만들어 reviewer prompt를 병렬 실행한다. 이 방식은 Codex subagent가 아니라
  `parallel reviewer threads`로 부른다.

- Intake: source inventory reviewer, claim/evidence reviewer, design reference
  reviewer.
- Strategy/spec: business structure reviewer, claim provenance reviewer, render
  feasibility reviewer.
- Build: HTML writer는 단일 owner로 둔다. copy/design/accessibility reviewer는
  읽기 전용 subagent로 병렬 실행한다.
- QA/revise: deterministic QA 후 visual/design/copy reviewers를 병렬 실행한다.
- Finalize: delivery checklist reviewer와 artifact freshness reviewer를 병렬 실행한다.

쓰기 권한은 stage owner 하나에게만 준다. reviewer subagent는 findings artifact만
작성하거나 parent thread에 결과를 반환한다. 서로 다른 agent가 같은 HTML을 직접
수정하지 않게 한다.

### Non-interactive mode

- CI나 batch deck 생성은 `slidex run --non-interactive`로 실행한다.
- 내부 Codex fallback은 `codex exec --json`을 사용하고 JSONL event를 보존한다.
- agent final output이 필요한 단계에는 `--output-schema`를 사용해
  `strategy_result.schema.json`, `deck_spec_patch.schema.json`,
  `review_findings.schema.json` 같은 stage schema를 요구한다.

### Review

- `slidex codex review`는 기본적으로 `turn/start + outputSchema`를 사용해 stage
  artifact에 대한 structured review findings를 생성한다.
- App Server `review/start`는 현재 git diff 또는 Codex native review UX가 필요한
  경우에만 보조로 실행한다. `review/start` 결과는 structured review schema로
  normalize한 뒤 gate에 반영한다.
- 문서 작성 후 최소 1회 GPT-5.5 Xhigh reviewer loop를 요구한다.
- reviewer가 blocker/major gap을 찾으면 parent stage가 보완하고 동일 범위를 다시
  review한다.
- reviewer가 "추가 보완 없음"을 명시하거나 남은 이슈가 accepted risk로 기록될
  때까지 반복한다.

### MCP

- `slidex codex doctor`는 `codex mcp list`와 App Server MCP status를 확인한다.
- project-scoped `.codex/config.toml`을 도입할 경우, source connector는
  `required = true` 여부를 명시해야 한다.
- OpenAI docs 같은 documentation MCP는 최신 기능 확인에 유용하지만, deck production
  자체의 필수 runtime dependency로 묶지 않는다.
- 장기적으로 `slidex mcp-server --stdio`를 제공해 Codex가 `inspect`, `render`,
  `qa`, `package`, `state/read` 같은 deterministic 기능을 구조화된 MCP tool로
  호출할 수 있게 한다. 이 MCP server는 production deck artifact를 직접 생성할 때도
  동일한 Go 구현을 호출해야 하며, 별도 코드 경로를 만들지 않는다.

### Skills, Plugins, Hooks

- `slidex`는 companion skill을 제공할 수 있다.
  - 목적: Codex가 `slidex` CLI를 언제/어떻게 호출해야 하는지 기억하게 한다.
  - 위치 후보: `.codex/skills/slidex/SKILL.md` 또는 배포용 skill package.
- plugin은 선택 사항이다. CLI 자체가 canonical interface이고, plugin은 Codex UI에서
  discoverability를 높이는 계층으로만 둔다.
- hooks는 위험한 직접 HTML 수정, dependency 변경, render output stale 상태를 감지해
  `slidex qa` 또는 `slidex package`를 권고하는 데 사용할 수 있다.

### Remote TUI와 App Server

- `slidex codex app-server start --listen ws://127.0.0.1:PORT`는 사용자가
  `codex --remote ws://127.0.0.1:PORT`로 같은 thread를 관찰하거나 이어받을 때만
  사용한다.
- 외부 interface bind는 기본 금지다. 허용 시 token file, SHA-256 verifier, issuer,
  audience, clock skew 설정을 모두 manifest에 기록한다.

### Visual Inputs

- App Server mode의 visual QA는 `turn/start.input`에 rendered PNG를
  `localImage` input으로 첨부한다. 로컬 `codex-cli 0.130.0` generated schema에서
  `UserInput` variant `localImage { type, path }`가 확인되었다.
- reviewer model은 `model/list`의 `inputModalities`가 `image`를 포함할 때만 visual
  review에 배정한다. 지원하지 않으면 visual review는 blocked finding으로 남긴다.
- fallback mode에서는 `codex exec --image <png>`를 사용한다.
- QA integration test는 최소 한 장의 rendered PNG가 App Server reviewer input으로
  전달되고, reviewer output이 image hash를 참조하는지 확인해야 한다.

## Pipeline 설계

### Stage Graph

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

모든 stage는 다음 contract를 따른다.

- inputs: file paths, hashes, Codex thread id, user answers
- outputs: expected artifacts, output hashes, machine-readable status
- verifier: Go function 또는 external command
- repair: 실패 시 실행할 next stage 또는 Codex prompt
- stop condition: pass, pass_with_risks, blocked, user_input_required

### Intake

`slidex intake`는 `prompts/_active_deck_context.md` 규칙을 Go 코드로 구현한다.

- deck 후보가 여러 개이면 한국어로 선택을 요청한다.
- `brief.md`, `DESIGN.md`, assets, brand, data, source, reference docs를 먼저
  inventory한다.
- 미완성 입력은 `out/intake_questions.md`에 질문으로 쓰고 멈춘다.
- `--answers FILE`을 주면 batch 모드로 brief를 갱신한다.

### Strategy/Spec/Build

- prompt template은 `internal/prompts/templates`에 vendoring하고 `go:embed`로
  포함한다.
- 각 stage는 App Server turn output을 output schema로 제한한다.
- `deck_spec.json`은 기존 schema로 검증하고, schema mismatch는 build 단계 진입을
  막는다.
- HTML build는 deterministic writer one-owner 원칙을 적용한다. 이후 reviewer는
  patch suggestion만 낸다.

### Render/QA/Package

- 기존 Go 구현을 `internal/render`, `internal/qa`, `internal/sync`로 분리한다.
- `render`는 `--deck`만으로 HTML/PDF/manifest/montage 기본 경로를 계산한다.
- `qa`는 deterministic findings와 Codex visual review findings를 병합하되,
  deterministic fail이 있으면 Codex review 통과만으로 pass 처리하지 않는다.
- `package`는 manifest freshness, PDF page count, rendered PNG blank check, QA
  report freshness, delivery summary existence를 모두 확인한다.
- 따라서 `delivery_summary`는 `package`보다 먼저 생성되어야 한다. 최종 `package`
  명령은 delivery summary까지 포함한 complete artifact set을 검증하는 마지막
  gate다. 기존 finalize prompt의 package-before-summary 안내는 CLI 전환 때 수정한다.
- `slidex finalize`는 `${OUT_DIR}/delivery_summary.md`를 작성하고, package가 검증할
  artifact list, QA status, accepted risks, review loop 결과, render manifest hash를
  요약한다.

### Migration

`slidex migrate`는 기존 workspace를 완전 CLI 모델로 올리는 명령이다.

- legacy root-level workspace를 `decks/<deck_id>/`로 옮기거나, root compatibility
  mode를 명시적으로 기록한다.
- historical PPTX 산출물은 generated deliverable이 아니라 archived artifact 또는
  source reference로 분류한다.
- 오래된 `deck_spec.json`의 PPTX/native PowerPoint field를 제거하거나 migration
  finding으로 표시한다.
- `final_deck.html`은 있지만 baseline이 없으면 사용자 확인 후
  `final_deck.generated_baseline.html`을 생성한다.
- `slidex_state.json`과 `codex_threads.json`이 없으면 초기화한다.
- migration은 기본 dry-run이며, `--write`를 줘야 파일을 바꾼다.

## 구성 파일

```toml
# slidex.toml
[workspace]
default_deck = "spikalabs"

[codex]
mode = "app-server"
required_version = "0.130.0"
model_strategy = "catalog-default"
analysis_model = "gpt-5.5"
analysis_reasoning_effort = "xhigh"
development_model = "gpt-5.5"
development_reasoning_effort = "high"
sandbox = "workspace-write"
approval_policy = "on-request"
enable_goals = true
enable_subagents = true

[render]
width = 1920
height = 1080
font_preset = "pretendard"
pdf_mode = "paginated"
```

원칙:

- Codex model catalog는 App Server `model/list`를 우선한다. 설정 파일의 model 값은
  사용자가 강제한 override일 때만 적용한다.
- exact pin 정책 때문에 `required_version`은 range가 아니라 정확한 버전 문자열이어야
  한다.
- `slidex doctor`는 `.mise.toml`, `go.mod`, Chrome version, Codex CLI version,
  feature flags, MCP status를 함께 검사한다.

## Exit Code와 출력

- `0`: pass
- `1`: fail finding 또는 command failure
- `2`: usage/config error
- `3`: user input required
- `4`: blocked by unsupported Codex/App Server feature
- `5`: stale artifacts
- `6`: accepted risks exist, but requested gate was strict pass

모든 명령은 기본 human-readable output을 제공하고 `--json`으로 machine-readable
output을 제공한다. 장기 명령은 progress를 stderr 또는 JSONL event stream으로 내고,
최종 artifact summary만 stdout에 쓴다.

## 보안과 격리

- App Server websocket은 localhost 외부에 노출하지 않는다.
- token file은 `0600` 권한을 요구한다.
- `thread/shellCommand`와 `process/spawn`처럼 sandbox 밖 실행 가능성이 있는 API는
  기본 비활성화한다. 필요한 경우 `slidex.toml`에서 stage별 allowlist로만 켠다.
- user-supplied source 문서는 deck workspace 밖으로 복사하지 않는다.
- generated artifact는 `${OUT_DIR}` 아래에만 쓴다.
- Codex approval policy는 stage별로 기록한다. irreversible action은 현재
  workflow에는 없어야 하며, 생기면 dry-run과 confirmation을 요구한다.

### State/Log 보안

`slidex_state.json`, `codex_threads.json`, `run_log.jsonl`, `agent_runs/`,
`agent_reviews/`는 사용자 답변, 사업 자료 요약, command output, MCP event,
local file path, model usage를 포함할 수 있다.

- 파일 권한은 기본 `0600`, 디렉터리는 `0700`으로 생성한다.
- token, bearer credential, cookie, API key, auth header, capability token 원문은
  절대 기록하지 않는다. token file path와 SHA-256 verifier만 허용한다.
- command output logging은 기본 summary mode로 제한하고, full event logging은
  `--verbose-log` 또는 config opt-in이 있을 때만 켠다.
- redaction rule은 `OPENAI_API_KEY`, `Authorization`, `Bearer`, `*_TOKEN`,
  `*_SECRET`, `cookie`, `set-cookie` 패턴을 기본 포함한다.
- `run_log.jsonl`과 `agent_runs/`는 delivery package에 기본 포함하지 않는다.
  필요한 경우 sanitized excerpt만 `delivery_summary.md`에 링크한다.
- repo `.gitignore` 또는 deck output ignore 정책에 state/log/cache artifact를
  명시한다. 사용자가 감사 목적으로 커밋하려면 `slidex package --include-logs`와
  sanitizer 통과가 필요하다.
- retention 정책을 둔다. 기본은 최신 successful run과 직전 failed run만 보존하고,
  `slidex clean --logs --older-than <duration>`로 정리한다.

## 마이그레이션 계획

### Phase 1: CLI 구조화

- `cmd/slidex/main.go`의 단일 파일 구현을 internal packages로 분리한다.
- 기존 6개 명령의 동작과 output contract를 유지한다.
- `--deck` 기반 default path resolution을 모든 명령에 추가한다.

### Phase 2: State와 Config

- `slidex.toml`, `${OUT_DIR}/slidex_state.json`,
  `${OUT_DIR}/codex_threads.json`, `${OUT_DIR}/run_log.jsonl`을 도입한다.
- `doctor`를 추가해 Go/mise/Codex/Chrome/MCP/schema 상태를 검사한다.
- `migrate`를 추가해 legacy root workspace, historical PPTX artifact, baseline
  누락, old spec field를 dry-run으로 진단한다.

### Phase 3: Prompt Template 내장

- 현재 `prompts/*.md`를 CLI template로 vendoring한다.
- `intake`, `strategy`, `spec`, `build`, `revise`, `finalize`, `clean` 명령을
  추가한다.
- prompt 실행 결과를 output schema로 검증한다.

### Phase 4: App Server Client

- `codex app-server` process manager와 JSON-RPC client를 구현한다.
- `thread/start`, `turn/start`, `thread/turns/list`, `thread/read`, `model/list`,
  `experimentalFeature/list`, `mcpServerStatus/list`, `review/start`,
  `thread/goal/*`를 지원한다.
- versioned schema generation과 compatibility gate를 추가한다.
- `exec_runner`도 같은 runner interface로 유지해 `--codex-mode exec`와 fallback을
  공식 지원한다.

### Phase 5: Full Run Orchestrator

- `slidex run`이 stage graph를 실행한다.
- stage skip/resume/retry/repair loop를 지원한다.
- App Server failure 시 `codex exec --json` fallback을 제공한다.

### Phase 6: Review와 Subagent Loop

- stage별 reviewer prompts와 findings schema를 만든다.
- 구조화 review는 `turn/start + outputSchema`를 primary path로 구현한다.
- Codex subagent mode와 parallel reviewer thread mode를 구분해 실행하고, 어떤
  모드였는지 review artifact에 기록한다.
- 병렬 reviewer를 실행하고, blocker/major finding이 사라질 때까지 보완 loop를 돈다.
- `${OUT_DIR}/agent_reviews/`에 round별 결과와 resolution을 기록한다.

### Phase 7: Documentation/Distribution

- README와 commands.md의 primary workflow를 `slidex run` 중심으로 바꾼다.
- 기존 `codex exec - < prompts/...` 명령은 advanced/manual fallback으로 내린다.
- PATH 설치, shell completion, release packaging, companion skill 문서를 추가한다.
- `slidex mcp-server`와 companion skill/plugin은 canonical CLI를 보조하는 배포
  계층으로 문서화한다.

## Acceptance Criteria

완료 조건은 다음을 모두 만족해야 한다.

- `slidex run --deck <deck>`만으로 intake gate부터 delivery summary까지 실행할 수
  있다. 자료가 부족하면 한국어 질문을 남기고 `user_input_required`로 멈춘다.
- 사용자가 prompt 파일을 직접 실행하지 않아도 모든 표준 stage를 CLI로 실행할 수
  있다.
- 기존 6개 deterministic 명령은 계속 동작한다.
- `slidex doctor --codex`가 Codex CLI version, App Server protocol schema,
  feature flags, goal availability, MCP status를 검사한다.
- App Server mode는 transport 연결 후 `initialize`, `initialized`, capability
  snapshot, `model/list`, `experimentalFeature/list`, `mcpServerStatus/list` probe를
  수행하고 실패를 structured finding으로 기록한다.
- App Server mode에서 `thread/start`, `turn/start`, streamed event logging,
  `thread/read` 또는 `thread/turns/list`, structured review turn, optional
  `review/start`, `thread/goal/*`가 stage state에 기록된다.
- `/goal`은 TUI slash command이고 `slidex goal`은 App Server goal API wrapper라는
  차이가 문서와 help에 명확하다.
- `slidex goal`은 objective non-empty 및 4,000자 이하 제약을 검증한다. 초과하면
  objective를 파일에 저장하고 파일 경로를 goal에 참조하라고 안내한다.
- `slidex goal pause/resume/complete`는 `thread/goal/set`에
  `paused/active/complete` status를 전달하고, `status`가 App Server와 local mirror의
  값을 함께 보여주는 integration test를 가진다.
- App Server가 불가능한 환경에서는 `codex exec --json` fallback이 동작하고
  기능 저하가 state와 QA report에 기록된다.
- `--non-interactive`는 App Server를 끄지 않는다. `exec`는 `--ci`, 명시적
  `--codex-mode exec`, 또는 App Server unavailable fallback에서만 기본 선택된다.
- structured reviewer loop가 최소 design, HTML, QA/delivery gate에서 실행 가능하다.
- Codex subagent mode를 쓴 경우 parent prompt, spawned item/thread evidence,
  collected result가 기록된다. parallel reviewer thread mode를 쓴 경우 subagent라고
  부르지 않고 별도 thread ids를 기록한다.
- visual QA는 App Server `localImage` input 또는 fallback `codex exec --image`로
  rendered PNG를 전달하고, integration test가 image attach 경로를 검증한다.
- reviewer가 발견한 blocker/major issue가 모두 해결되거나 accepted risk로
  남겨진다.
- fixture deck 기준 `slidex run --deck <fixture> --until package --codex-mode app-server`
  e2e가 HTML, baseline, rendered PNG, PDF, render manifest, QA montage, QA report,
  delivery summary, package pass status를 생성하고 검증한다.
- 같은 fixture에서 App Server 강제 실패를 주입하면 `codex exec --json` fallback이
  실행되고 fallback risk가 state와 QA report에 기록된다.
- `slidex migrate --deck <deck> --dry-run`이 legacy root/PPTX/baseline/spec-field
  상태를 수정 없이 보고한다.
- stale artifact detection, resume/retry, concurrent run lock, app-server PID/socket
  lifecycle, state/log redaction이 테스트된다.
- `slidex package --include-logs`는 sanitizer를 통과한 log만 포함하고,
  `slidex clean --logs --older-than <duration>`은 retention 정책에 따라 로그를
  정리한다.
- HTML/PDF workflow의 기존 산출물 계약은 깨지지 않는다.
- PPTX는 생성/선택 납품물로 다시 등장하지 않는다.
- `mise exec -- go test ./...`가 통과한다.

## 열린 결정 사항

- App Server client를 Go에서 직접 구현할지, TypeScript Codex SDK adapter를 별도
  child process로 둘지 결정해야 한다. 로컬 CLI 단일 바이너리 목표에는 Go direct
  client가 맞지만, SDK가 더 안정적인 API surface를 제공하면 adapter가 실용적일 수
  있다.
- Codex model selection은 `model/list`의 default를 따를지, `slidex.toml`에서
  `gpt-5.5`를 강제할지 결정해야 한다. 프로젝트 지침은 설계/분석에 GPT-5.5 Xhigh,
  개발에 GPT-5.5 High를 요구하므로, local catalog가 이를 제공하지 않을 때의 fallback
  정책이 필요하다.
- companion skill/plugin을 이 저장소에 포함할지, 별도 배포물로 관리할지 결정해야
  한다.

## Reviewer Loop 기록

- Round 1: GPT-5.5 Xhigh reviewer found no blockers, but identified major gaps
  in App Server default mode, initialize handshake, review runner structure,
  subagent terminology, visual input handling, state/log security, lifecycle,
  goal validation, and e2e acceptance. These findings were incorporated in this
  revision.
- Round 2: GPT-5.5 Xhigh reviewer found no blockers, but identified remaining
  major gaps in exact App Server method names/values, delivery summary command
  closure, e2e acceptance, and stdio/shared server lifecycle separation, plus
  minor gaps in log-cleaning command surface and goal status acceptance. These
  findings were incorporated in this revision.
- Round 3: GPT-5.5 Xhigh reviewer found no blocker, major, or minor findings and
  confirmed that the Round 1/2 findings were sufficiently closed.
