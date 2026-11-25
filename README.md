# lazykcp

A read-only terminal UI for browsing kcp workspaces and the API resources available in each workspace.

## Run

The kubeconfig's current context must point to a kcp workspace endpoint under `/clusters/<workspace-path>`. The application starts from that workspace and uses its credentials for all workspace requests.

```sh
go run . --kubeconfig ~/.kube/config
```

If `--kubeconfig` is omitted, the standard Kubernetes client-go kubeconfig loading rules are used.

For a local kcp endpoint with a self-signed certificate, you can explicitly bypass certificate verification:

```sh
go run . --kubeconfig ~/.kube/config --insecure-skip-tls-verify
```

This option is off by default. It keeps traffic encrypted but disables verification of the server's identity, making connections vulnerable to interception. When enabled, any CA configured in kubeconfig is ignored. Prefer configuring the certificate authority in kubeconfig, and use the bypass only for local development.

## Navigation

- The workspace tree spans the top. Below it, API resources and instances share a narrow navigation rail beside the detail view. On narrower terminals, those navigation panes move above the full-width detail view.
- The UI needs at least a 40x16 terminal; narrower layouts are supported above that size.
- Select a workspace, resource, or instance with `Enter`; use `Right`/`l` to expand a workspace and `Left`/`h` to collapse or move to its parent.
- Move with `j`/`k` or the arrow keys; use `Ctrl+D`/`Ctrl+U` to move half a pane at a time. Use `Tab`/`Shift+Tab` to change panes; `h`/`l` also changes panes outside the workspace tree.
- Press `/` in a sidebar to filter its entries; press `/` in the detail pane to search the current representation. Use `n`/`N` to move between matches.
- Use `1`/`[` for JSON and `2`/`]` for YAML. Press `q` to exit.
- Press `v` to hide or restore the resource/instance navigation panes while keeping the workspace tree visible.
- Use the mouse wheel over a pane to focus and scroll it. Click a workspace, resource, or instance row to activate it; click a workspace `+`/`-` marker to expand or collapse that branch. Click the JSON/YAML tabs in the detail pane's top border to switch representations. Drag pane borders to resize the visible splits for the current run.
- A thumb on a pane's right border indicates the current position when its content extends beyond the visible area.

The configured identity needs permission to list workspaces and the resources you want to inspect. Resource discovery and instance listing use the permissions available in the selected workspace. The initial version does not create, update, or delete resources.
