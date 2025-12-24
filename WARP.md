# WARP.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Overview
This repository generates Cisco AO (Automation Orchestration) Atomics from OpenAPI specifications. The primary use case is generating workflow JSON from Meraki and NetBox OpenAPI specs. The generator parses OpenAPI operations, creates input/output variables, and produces complete workflow JSON with idempotency support and categorization.

## Key Commands

### Build Commands
```bash
# Build for current platform
go build -o generate_workflow generate_workflow.go

# Build universal macOS binary (ARM64 + AMD64)
bash ./generate_executable.sh

# Build for specific platform
GOOS=darwin GOARCH=arm64 go build -o bin/generate_workflow
```

### Run Commands
```bash
# Generate single workflow from operationId
./generate_workflow \
  -openapi=/path/to/spec3.json \
  -operationId=createOrganizationNetwork \
  -platform=Meraki \
  -supportIdempotency=true \
  -idempotencyCondition="Name has already been taken" \
  -categoryId=category_02LJAJ25TKWKJ2JWbrsG5z2UzO6wBxu5BLi \
  -categoryName="Cisco Meraki - Wireless"

# Generate from config file (batch mode)
./generate_workflow \
  -openapi=/path/to/netbox-openapi.yaml \
  -config=workflow-config.yaml \
  -outputDir=outputs \
  -connector=netbox

# Using go run during development
go run generate_workflow.go -openapi=specs/netbox-openapi.yaml -operationId=dcim_devices_list -connector=netbox
```

### Test Commands
```bash
# Run all tests
go test ./...

# Run specific test
go test -run TestWorkflowGeneration ./...

# Run tests with verbose output
go test -v ./...
```

### Dependencies
```bash
# Install/update dependencies
go mod download
go mod tidy
```

## Architecture

### Core Flow
1. **OpenAPI Parsing**: `ExtractOperation()` locates the operation by ID across all HTTP methods (GET/POST/PUT/DELETE)
2. **Schema Resolution**: `resolveOperationSchemas()` recursively follows `$ref` pointers in the OpenAPI spec to expand schemas
3. **Variable Generation**: Path/query params and request body properties become workflow input variables
4. **Template Rendering**: `workflowTemplate` (Go text/template) generates the final workflow JSON with KSUID placeholders
5. **KSUID Replacement**: `ReplaceKSUIDs()` ensures unique IDs across workflow components

### Connector System
The generator supports multiple connectors (platforms) through the `connectorConfig` abstraction:
- **Meraki**: Uses `meraki.api_request` action type, `/api/v1` base path
- **NetBox**: Uses `netbox.invoke_api` action type, no base path, generates Python script for query string building

Each connector defines:
- `AtomicGroup`: Workflow atomic group name
- `TargetType`: Runtime endpoint type (e.g., `meraki.endpoint`, `netbox.endpoint`)
- `ActionType`: API request action type
- `ResponseBodyField`: Where to find response body in action output
- `BuildActionProps`: Function to construct connector-specific action properties

### Variable Generation
Variables are created from three sources:
1. **Path Parameters**: Hidden inputs (`Input - <Name>`), always required
2. **Query Parameters**: Visible wizard inputs (`Query - <Name>`), follow OpenAPI required flags
3. **Request Body Properties**: Visible inputs (`Input - <Name>`) from POST/PUT body schemas

Output variables are generated from response schema properties.

### Idempotency Logic
When `-supportIdempotency=true`:
- Adds boolean input variable (`Input - Ignore If Exists`)
- POST: Checks error message against `-idempotencyCondition` pattern
- DELETE/GET/PUT: Checks status code against condition (typically "404")
- Injects conditional logic blocks that complete successfully when idempotency triggers

### Acronym Normalization
`capitalizeAcronyms()` loads `networking_acronyms.csv` and applies regex replacements to workflow/variable/action names to ensure consistent capitalization (e.g., "Vlan" → "VLAN").

## Configuration Files

### workflow-config.yaml
Batch workflow generation config with:
- `defaults.query_params`: Global query params for all GET endpoints
- `workflows[].endpoint`: OpenAPI path (e.g., `/dcim/devices`)
- `workflows[].methods`: List of HTTP methods to generate
- `workflows[].query_params`: Endpoint-specific allowed query params (filters spec params)
- `workflows[].body_params`: POST/PUT body properties to expose (filters large schemas)
- `workflows[].options`: Per-workflow overrides for idempotency, category, platform

### networking_acronyms.csv
Single-row CSV with networking acronyms (e.g., `VLAN,API,IP,DNS`). Used by `capitalizeAcronyms()` to normalize terminology across generated names.

## Important Flags

### Required Flags
- `-openapi`: Path to OpenAPI spec (JSON or YAML)
- `-operationId`: Target operation from spec (unless using `-config`)
- `-config`: Batch mode config file (replaces `-operationId`)

### Platform Flags
- `-connector`: Target platform (`meraki` or `netbox`, default: `meraki`)
- `-platform`: Display name prefix for workflows (default: connector's platform name)

### Idempotency Flags
- `-supportIdempotency`: Enable idempotency logic (default: `false`)
- `-idempotencyCondition`: Error message regex (POST) or status code (DELETE/GET/PUT)

### Category Flags
- `-categoryId`: AO category ID for workflow placement
- `-categoryName`: Category display name

### Advanced Flags
- `-stringifyBodyInputs`: Force numeric/boolean body inputs to strings (connector workaround)
- `-queryParamsConfig`: JSON/YAML file mapping operationIds to allowed query params
- `-outputDir`: Output directory for `-config` mode (default: `outputs`)

## Special Handling

### NetBox Specifics
- **Query String Building**: NetBox GET endpoints with query params generate Python scripts to URL-encode parameters
- **Pagination Schema**: List endpoints automatically get `count`, `next`, `previous`, `results` output variables
- **Default Filters**: `dcim_devices_list` has hardcoded default query params (`q`, `name`, `id`, etc.)

### Schema Overrides
`applyOperationSchemaOverrides()` patches specific operations:
- `ipam_prefixes_available_prefixes_create`: Adds `prefix_length` property, removes `prefix` from required list

### Body/Query Param Filtering
- `body_params` in config: Only specified properties appear in request body template
- `query_params` in config: Only specified query params generate input variables
- Always includes `q` parameter for NetBox list endpoints (unless explicitly filtered)

## Output Structure
Generated workflow JSON contains:
- `workflow.unique_name`: KSUID-based unique identifier
- `workflow.variables`: Input/output variable definitions
- `workflow.properties`: Atomic group, target type, runtime user config
- `workflow.actions`: API request, conditional logic, JSONPath queries, completion actions
- `categories`: Category metadata for AO UI placement

Workflows must be manually validated in AO after generation, especially:
- Runtime user wiring
- Target metadata correctness
- Idempotency message accuracy

## Project Structure
- `generate_workflow.go`: Houses the CLI entrypoint, OpenAPI parsing, template creation, and acronym normalization
- `generate_executable.sh`: Builds universal macOS binary using `lipo` to merge ARM64 and AMD64 builds
- `networking_acronyms.csv`: Vendor terminology loaded at runtime by `capitalizeAcronyms()`
- `workflow-config.yaml`: Batch generation configuration
- `specs/`: OpenAPI specification files
- `outputs/`: Default directory for generated workflows

When adding new functionality, place shared helpers under `pkg/` or `internal/` to keep the root uncluttered.

## Coding Style
- Run `gofmt` (or `goimports`) before every commit
- Use tabs for indentation
- Keep imports alphabetical
- Avoid lines beyond ~100 characters
- Exported types (e.g., `WorkflowData`) use PascalCase
- JSON tags use snake_case
- Generated wizard labels follow `Input - Foo` / `Query - Bar` pattern
- Use `golangci-lint` or `staticcheck` when available

## Testing Guidelines
- Create table-driven tests in `*_test.go` files
- Feed trimmed OpenAPI fixtures and compare against golden workflow JSON in `testdata/`
- Run `go test ./...` for full suite
- Run `go test ./... -run TestWorkflowGeneration` to narrow focus
- Manual verification in AO is required—maintain checklist covering:
  - Idempotency messages
  - Runtime user wiring
  - Target metadata

## Commit Guidelines
- Use Conventional Commits format (e.g., `feat: add vlan delete workflow`, `fix: normalize acronym case`)
- Keep commits focused: generator logic, template updates, and CSV adjustments should land separately
- Pull requests should:
  - Describe affected operationIds
  - Include exact command used with flags
  - Provide before/after JSON snippets or AO screenshots
  - Mention any manual post-merge steps

## Security Notes
- Strip internal hostnames or tenant identifiers from OpenAPI samples before committing
- Do not embed credentials in templates—use AO runtime users
- Build binaries from clean tree (`git status` empty) and document GOOS/GOARCH

## Development Notes
- Templates use `$WorkflowKSUID`, `$ApiRequestKSUID`, etc. as placeholders—replaced by `ReplaceKSUIDs()` post-render
- OpenAPI YAML specs are auto-converted to JSON via `sigs.k8s.io/yaml`
- The generator only supports OpenAPI 3.x specs
- Missing flag values (e.g., `-platform`, `-categoryId`) silently drop wizard variables or categories from rendered JSON
- Document new flags in README.md alongside sample invocations
