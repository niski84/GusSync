# Wails Build Issues & Solutions

## Current Issue

Wails `dev` is failing during binding generation. The error shows it's trying to compile CLI `main.go` which requires flags.

## Root Cause

We have two `main()` functions:
1. `main.go` - CLI entry point (requires `-source`, `-dest` flags)
2. `wails_main.go` - Wails entry point (calls `app.Run()`)

Wails is compiling the CLI `main.go` during binding generation.

## Attempted Solution: Build Tags

Added build tags:
- `main.go`: `//go:build !wails`
- `wails_main.go`: `//go:build wails`

This should exclude CLI main when building with `-tags wails`, but Wails may not be using the build tag during binding generation.

## Next Steps

1. **Test if build tags work:**
   ```bash
   go build -tags wails -o /tmp/test_wails
   ```

2. **Check Wails configuration:**
   - Wails may need explicit build tag in `wails.json`
   - Or we may need to exclude `main.go` from Wails builds

3. **Alternative: Rename CLI main:**
   - Move CLI code to `cli/` subdirectory
   - Keep Wails code at root or in `app/`

## Status

Build tags added but not yet verified with Wails CLI. Embed path issue also needs verification (frontend/dist must exist).


