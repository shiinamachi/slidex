param(
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]] $RemainingArgs
)

$ErrorActionPreference = "Stop"
& slidex doctor --codex --render --json @RemainingArgs
exit $LASTEXITCODE
