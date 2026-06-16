# Contributing to XEME OS

Thanks for wanting to make this better. Here's how.

## TL;DR

```bash
# Fork the repo, then:
git clone https://github.com/YOUR_USERNAME/xeme-os.git
cd xeme-os
go test ./...
make build   # or: for c in xeme xeme-os xeme-mcp; do go build -o $c ./cmd/$c; done

# Make your change, write a test, push, open a PR.
```

## How to add a new engine

Each engine is a Go package under `internal/`. The pattern is:

```go
// internal/myengine/myengine.go
package myengine

type Engine struct { /* config + http client */ }
func New(cfg *Config) *Engine { /* ... */ }
func (e *Engine) DoSomething(ctx, args) (*Result, error) { /* ... */ }
func (e *Engine) Version() string { return "xeme-myengine v1.0.0" }
func (e *Engine) Health() error { return nil }
```

Then wire it into the dashboard and MCP server:

1. `cmd/xeme-os/main.go` → add to `getAllEngineStatus`
2. `cmd/xeme-mcp/main.go` → add tool definitions + handler
3. `cmd/xeme/main.go` → add CLI subcommand

## How to add a new MCP tool

In `cmd/xeme-mcp/main.go`:

```go
// 1. Add to `tools` array:
{Name: "xeme_myengine_do_something", Description: "...", InputSchema: ...}

// 2. Add a case in callTool():
case "xeme_myengine_do_something": return toolMyEngineDoSomething(args)

// 3. Implement the function:
func toolMyEngineDoSomething(args map[string]interface{}) (interface{}, error) { ... }
```

## Code style

- Go 1.23+
- `gofmt -s` (CI enforces this)
- `go vet ./...` must pass
- One package per engine
- Public functions get doc comments
- No external deps in `internal/` unless necessary — stdlib first
- Tests: at least one happy-path test per public function

## Commit messages

```
feat: add MoltSets enrichment (closes #12)
fix: handle empty CSV in xeme enrich
docs: update README with new install command
refactor: extract AEO config to a struct
test: add cost cap test for TheirStack
chore: bump go.mod to 1.23
```

## PR checklist

- [ ] `gofmt -s` clean
- [ ] `go vet ./...` passes
- [ ] `go test ./...` passes
- [ ] New public function has a doc comment
- [ ] New public function has at least one test
- [ ] README updated if you added a new command / tool
- [ ] No secrets committed (run `git diff --cached | grep -i "api[_-]key"`)

## Architecture rules

- `internal/` packages **must not** import `cmd/`
- `cmd/xeme-mcp/` and `cmd/xeme-os/` may share helpers via `internal/`
- `internal/<engine>/` should be self-contained — its own types, its own client, its own tests
- Cross-cutting config goes in `internal/config/` or as env vars

## Where to ask

- GitHub Discussions: https://github.com/rasulshaikh/xeme-os/discussions
- GitHub Issues: https://github.com/rasulshaikh/xeme-os/issues

## License

By contributing, you agree your code is MIT-licensed (same as the project).
