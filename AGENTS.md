# Repository guidance

## Project

`lazykcp` is a read-only Go terminal UI for browsing kcp workspaces, discovering API resources, and inspecting resource instances. It uses Go 1.24, Bubble Tea/Lipgloss for the terminal UI, and Kubernetes `client-go` for API access.

## Build, test, and run

- Build: `go build ./...`
- Run all tests: `go test ./...`
- Run one test: `go test ./internal/ui -run '^TestSidebarFiltersAndDetailRepresentations$' -count=1` (replace the test name or package as needed)
- Static analysis: `go vet ./...`
- Format Go files: `gofmt -w <files>`
- Run against the current kubeconfig context: `go run . --kubeconfig ~/.kube/config`

The current kubeconfig server URL must include `/clusters/<workspace-path>`. The optional `--insecure-skip-tls-verify` flag is for local development only; keep verification enabled by default.

## Architecture

- `main.go` parses kubeconfig and TLS options, creates the kcp client, then starts the Bubble Tea program.
- `internal/kcp` derives the starting workspace from the kubeconfig server URL. It clones the base REST config for each workspace path, uses the dynamic client to list workspaces and instances, and uses Kubernetes discovery to find listable resources. The API layer only reads data.
- `internal/ui` owns the Bubble Tea model: workspace tree, selected resource and instance, filters, search, and rendering. API calls run as Bubble Tea commands and return typed result messages; results are checked against the active workspace and selection so stale requests do not overwrite newer state.
- The UI adapts its pane arrangement to terminal dimensions. `layoutForView` informs both rendering and mouse hit-testing; keep those coordinate calculations consistent when changing layouts.

## Codebase conventions

- Keep Kubernetes objects as `unstructured.Unstructured` and identify resources by their group/version/resource tuple; the API served by each workspace is discovered at runtime.
- Workspace API paths are colon-delimited (`parent:child`). Per-workspace requests must use a copied REST config so changing the host path does not mutate the base kubeconfig config.
- Discovery excludes subresources and resources without the `list` verb. Workspace, resource, and instance results are sorted for stable navigation.
- Wrap API/configuration errors with operation and workspace context, then display them through the UI status state.
- TLS verification is enabled unless explicitly opted out. When the unsafe bypass is enabled, clear configured CA roots as well as enabling insecure verification.
- Tests live beside their package as `*_test.go`. The kcp tests cover config/path behavior; UI tests exercise model state, keyboard/mouse handling, and rendered layout without requiring a live kcp server.
