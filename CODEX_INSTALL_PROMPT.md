# Codex App One-Shot Install Prompt

## How to use / 사용법

Choose one release channel, then copy the matching prompt into a **Codex App**
chat. Codex will read the repository install guide, install the `slidex` CLI
and Codex plugin, and verify the setup.

설치 채널을 하나 선택한 뒤, 해당 프롬프트를 **Codex App** 채팅창에 붙여넣으세요.
Codex가 저장소 설치 가이드를 읽고 `slidex` CLI와 Codex 플러그인을 설치한 뒤
검증까지 수행합니다.

## Production Prompt

```text
Install slidex production build from https://github.com/shiinamachi/slidex. Read INSTALL.md in that repository and follow the production channel install instructions.
```

## Canary Prompt

```text
Install slidex canary build from https://github.com/shiinamachi/slidex. Read INSTALL.md in that repository and follow the canary channel install instructions; keep canary separate from any existing production install.
```

## What this prompt does / 이 프롬프트가 수행하는 작업

| Step | Description |
|------|-------------|
| 1 | Read `INSTALL.md` from `https://github.com/shiinamachi/slidex` |
| 2 | Use the requested production or canary channel |
| 3 | Download the matching release package and checksum file |
| 4 | Verify the SHA-256 checksum, install the CLI, and register the bundled Codex plugin |
| 5 | Run the install verification commands from `INSTALL.md` and report the result |

Default installation does not require GitHub CLI or GitHub login. If `gh` is
already installed and authenticated, or if the user explicitly asks for stronger
release provenance verification, `INSTALL.md` includes an optional GitHub
artifact attestation check.
