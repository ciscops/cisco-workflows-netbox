# Repository Guidelines

## Project Structure & Module Organization
`generate_workflow.go` houses the CLI entrypoint, OpenAPI parsing, template creation, and acronym normalization. Ship the compiled binary as `generate_workflow`; `generate_executable.sh` invokes two platform builds and merges them with `lipo`. Keep vendor terminology synced inside `networking_acronyms.csv`, since `capitalizeAcronyms` loads it at runtime. Add shared helpers under `pkg/` or `internal/` to keep the root uncluttered, and document new flags in `README.md` alongside sample invocations.

## Build, Test, and Development Commands
```bash
go run generate_workflow.go -openapi=/path/spec3.json -operationId=createOrganizationNetwork
GOOS=darwin GOARCH=arm64 go build -o bin/generate_workflow
bash ./generate_executable.sh   # universal macOS binary
go test ./...
```
Pass `-platform`, `-supportIdempotency`, `-idempotencyCondition`, and category flags exactly as shown above; missing values silently drop wizard variables or categories from the rendered workflow JSON.

## Coding Style & Naming Conventions
Run `gofmt` (or `goimports`) before every commit and keep imports alphabetical. Use tabs, avoid lines beyond ~100 characters, and let `golangci-lint` or `staticcheck` guard more advanced rules when available. Exported types such as `WorkflowData` remain PascalCase, JSON tags stay snake_case, and generated wizard labels follow the `Input - Foo` / `Query - Bar` pattern so AO surfaces stay predictable.

## Testing Guidelines
Create table-driven tests beside the generator in `*_test.go` files, feeding trimmed OpenAPI fixtures and comparing against golden workflow JSON under `testdata/`. Run `go test ./...` for the suite or `go test ./... -run TestWorkflowGeneration` to narrow focus. Because workflows must still be validated in AO, attach a manual verification checklist covering idempotency messages, runtime user wiring, and target metadata.

## Commit & Pull Request Guidelines
Adopt Conventional Commits (`feat: add vlan delete workflow`, `fix: normalize acronym case`) to make change logs readable. Keep commits focused: generator logic, template updates, and CSV adjustments should land separately. Pull requests must describe affected operationIds, paste the exact command used (with flags), and provide before/after JSON snippets or AO screenshots. Mention any manual post-merge steps such as rebuilding binaries.

## Security & Configuration Notes
Strip internal hostnames or tenant identifiers from OpenAPI samples before committing. Do not embed credentials in templates; lean on AO runtime users configured in the platform. When sharing binaries, build from a clean tree (`git status` empty) and document the GOOS/GOARCH inputs so other agents can reproduce the artifact.
