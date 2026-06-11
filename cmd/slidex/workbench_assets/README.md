# Workbench Frontend Assets

These files are generated from the repo-managed SolidStart CSR package in
`workbench/` and embedded into the slidex Go binary. Installed release packages
serve these local assets from the loopback Workbench without requiring Node or
pnpm at runtime.

Regenerate them with:

```bash
mise exec -- pnpm build
```

The build writes `slidex-workbench-build.json`, which records the generated
entry scripts and the Workbench source hash used for freshness checks.
