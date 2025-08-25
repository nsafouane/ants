# Crush Development Guide

## Build/Test/Lint Commands

- **Build**: `go build .` or `go run .`
- **Test**: `task test` or `go test ./...`
  - Run single test: `go test ./path/to/package -run TestName`
  - Run single test with verbose output: `go test ./path/to/package -run TestName -v`
- **Update Golden Files**: `go test ./... -update`
  - Update specific package: `go test ./internal/tui/components/core -update`
- **Lint**: `task lint` (check) or `task lint-fix` (fix)
- **Format**: `task fmt` (runs gofumpt)
- **Dev**: `task dev` (runs with profiling)

## Code Style Guidelines

- **Imports**: Use goimports formatting; group by stdlib, external, internal
- **Formatting**: Use gofumpt (stricter than gofmt); enabled in .golangci.yml
- **Naming**: Standard Go conventions - PascalCase exported, camelCase unexported
- **Types**: Prefer explicit types; use type aliases for clarity (e.g., `type AgentName string`)
- **Error handling**: Return errors explicitly; use `fmt.Errorf` with %w for wrapping
- **Context**: Always pass context.Context as first parameter for operations
- **Interfaces**: Define in consuming packages; keep small and focused
- **Structs**: Use embedding for composition; group related fields
- **Constants**: Use typed constants with iota for enums; group in const blocks
- **Testing**: Use testify's `require`; parallel tests with `t.Parallel()`
  - Use `t.SetEnv()` for environment variables
  - Use `t.TempDir()` for temporary directories (auto-cleaned)
- **JSON tags**: Use snake_case for JSON field names
- **File permissions**: Use octal notation (0o755, 0o644)
- **Comments**: End full-line comments with periods; inline comments without

- **Agent Rule**: Before generating code, you MUST fetch the latest documentation for libraries, frameworks, or any technology it will use via Context7 MCP.

## Workflow

- **Design Document**: You must read the `C:\Users\Dell\Desktop\CodeMem\Ants plan\Context_engine.md` file as a design document for the implementation.
- **Implementation Checklist**: The file `C:\Users\Dell\Desktop\CodeMem\Ants plan\CONTEXT_ENGINE_IMPLEMENTATION_PLAN.md` serves as a step-by-step implementation checklist. You should follow this plan task by task and update the task as complete `[x]` when confident that the task is complete.
