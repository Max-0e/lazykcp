# lazykcp

A lightweight terminal UI for browsing kcp workspaces and their API resources, with an embedded shell scoped to the selected workspace.

## Install

The installer downloads the latest release for Linux or macOS (amd64 or arm64), verifies its SHA-256 checksum, and places `lazykcp` in `$HOME/.local/bin` without requiring `sudo`.

```sh
curl -fsSL https://raw.githubusercontent.com/Max-0e/lazykcp/main/install.sh | bash
```

Add the install directory to your `PATH` if the installer reports that it is not already present:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

To install a specific release or use a different destination, set environment variables on the installer process:

```sh
curl -fsSL https://raw.githubusercontent.com/Max-0e/lazykcp/main/install.sh | LAZYKCP_VERSION=v1.0.0 INSTALL_DIR="$HOME/bin" bash
```

## Releasing

Push a version tag such as `v1.0.0` to trigger the release workflow. It runs the Go tests and vet checks, then publishes checksum-verified Linux and macOS binaries for amd64 and arm64 as GitHub release assets.

## Run

The kubeconfig's current context must point to a kcp workspace endpoint under `/clusters/<workspace-path>`. The application starts from that workspace and uses its credentials for all workspace requests.

```sh
lazykcp --kubeconfig ~/.kube/config
```

If `--kubeconfig` is omitted, the standard Kubernetes client-go kubeconfig loading rules are used.
To run from a source checkout instead, use `go run . --kubeconfig ~/.kube/config`.

For a local kcp endpoint with a self-signed certificate, you can explicitly bypass certificate verification:

```sh
go run . --kubeconfig ~/.kube/config --insecure-skip-tls-verify
```

This option is off by default. It keeps traffic encrypted but disables verification of the server's identity, making connections vulnerable to interception. When enabled, any CA configured in kubeconfig is ignored. Prefer configuring the certificate authority in kubeconfig, and use the bypass only for local development.

## Navigation

- The workspace tree spans the top. Below it, API resources and instances share a narrow navigation rail beside the detail view. On narrower terminals, those navigation panes move above the full-width detail view.
- The UI needs at least a 40x16 terminal; narrower layouts are supported above that size.
- Select a workspace, resource, or instance with `Enter`; use `Right`/`l` to expand a workspace and `Left`/`h` to collapse or move to its parent. `▶` and `▼` indicate collapsed and expanded workspace branches and API groups.
- Move with `j`/`k` or the arrow keys; use `Ctrl+D`/`Ctrl+U` to move half a pane at a time. Use `Tab`/`Shift+Tab` or `h`/`l` to change panes; on a focused API-group heading, `h` collapses and `l` expands it.
- Press `/` in a sidebar to filter its entries; press `/` in the detail pane to search the current representation. Use `n`/`N` to move between matches.
- Details opens with an instance-focused Overview showing identity, metadata, optional status and conditions, and a compact summary of spec fields. The Overview uses only the selected object already loaded by the UI. With Details focused, press `[`/`]` to cycle backward/forward through Overview, JSON, and YAML; `o`, `1`, and `2` select those views directly. The tabs are also clickable. Search and scrolling apply to the selected representation.
- With API resources focused, use `[`/`3` for Flat and `]`/`4` for Group. The bracket keys are scoped to the focused pane.
- The Group view starts with group headings collapsed; core API resources appear under `core`. Use `j`/`k` or the arrow keys to navigate headings and resources. Press `Enter` or click a group heading to expand/collapse it; use `h`/`l` or `Left`/`Right` on a heading to collapse/expand it. Nested resources are indented beneath their group.
- Press `q` to exit.
- Press `t` while the Workspaces pane is focused to open an embedded shell scoped to the highlighted workspace. It uses the same kubeconfig identity, starts in the application's current directory, and returns to browsing when the shell exits (normally `Ctrl+D`). No Git checkout or worktree is created.
- While the shell is active, it occupies the Details pane; the workspace, API resources, and instances panes remain visible. Shell keyboard input is focused in the Details pane.
- The browser panes are read-only, but the embedded shell can run arbitrary commands with your kubeconfig permissions, including commands that modify or delete cluster resources. Review commands before running them; lazykcp does not restrict shell capabilities.
- Press `v` to hide or restore the resource/instance navigation panes while keeping the workspace tree visible.
- Use the mouse wheel over a pane to focus and scroll it. Click a workspace, resource, or instance row to activate it; click a workspace `▶`/`▼` marker to expand or collapse that branch. Click the Flat/Group tabs in the API resources pane and the Overview/JSON/YAML tabs in Details to switch views. Drag pane borders to resize the visible splits for the current run.
- A thumb on a pane's right border indicates the current position when its content extends beyond the visible area.

The configured identity needs permission to list workspaces and the resources you want to inspect. Resource discovery and instance listing use the permissions available in the selected workspace. The initial version does not create, update, or delete resources.
