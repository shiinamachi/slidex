# Codex App One-Shot Install Prompt

## How to use / 사용법

Copy the prompt below and paste it into a **Codex App** chat. Codex will
automatically clone the repository, install the `slidex` CLI and Codex plugin,
and verify the setup.

아래 프롬프트를 **Codex App** 채팅창에 붙여넣으세요. Codex가 자동으로 저장소를
클론하고, `slidex` CLI와 Codex 플러그인을 설치한 뒤 검증까지 수행합니다.

## Prompt

```text
Install slidex from https://github.com/shiinamachi/slidex; read INSTALL.md in that repository and complete every step: detect the local OS and architecture, confirm GitHub CLI is available for artifact attestation verification, download the matching release package from the latest GitHub Release tag, verify the SHA-256 checksum and GitHub artifact attestation, extract and install the binary to a stable directory, add it to PATH, register the Codex plugin from the bundled marketplace, restart Codex, start a new Codex thread, and run "slidex --help", "slidex update status --json", and "slidex doctor --render" to confirm the CLI. If update status reports pendingActivation, run the reported pendingActivationCommand before plugin smoke. Run "slidex codex app-server plugin-smoke --json", and then run "slidex update verify --json" to confirm bundled plugin skills match the install. If update status reports restartRequired, restart Codex, start a new thread, rerun "slidex codex app-server plugin-smoke --json", and then rerun "slidex update verify --json" before treating bundled skills as active. Report each step's result.
```

## What this prompt does / 이 프롬프트가 수행하는 작업

| Step | Description |
|------|-------------|
| 1 | Clone or fetch the `shiinamachi/slidex` repository |
| 2 | Read `INSTALL.md` for detailed internal instructions |
| 3 | Detect OS (`linux`, `darwin`, `windows`) and CPU architecture (`amd64`, `arm64`) |
| 4 | Resolve the latest GitHub Release tag |
| 5 | Confirm GitHub CLI is available, then download the matching release package and checksum file |
| 6 | Verify the SHA-256 checksum and GitHub release/artifact attestations |
| 7 | Extract and install the `slidex` binary to a stable directory |
| 8 | Add the install directory to `PATH` |
| 9 | Register the Codex plugin from the bundled `.agents/plugins/marketplace.json` |
| 10 | Restart Codex, start a new thread, then run `slidex --help`, `slidex update status --json`, and `slidex doctor --render` to verify the CLI |
| 11 | If `pendingActivation` is true, run the reported `pendingActivationCommand` |
| 12 | Run `slidex codex app-server plugin-smoke --json`, and then run `slidex update verify --json` |
| 13 | If `restartRequired` is true, restart Codex, start a new thread, rerun plugin smoke, and rerun `slidex update verify --json` |
