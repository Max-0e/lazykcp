package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Max-0e/lazykcp/internal/kcp"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestGroupedResourceViewPreservesSelectionAndCollapsesGroups(t *testing.T) {
	model := testModel(t)
	model.resources = []kcp.Resource{
		{Name: "deployments", Group: "apps", Version: "v1", Kind: "Deployment"},
		{Name: "pods", Group: "", Version: "v1", Kind: "Pod"},
		{Name: "jobs", Group: "batch", Version: "v1", Kind: "Job"},
		{Name: "replicasets", Group: "apps", Version: "v1", Kind: "ReplicaSet"},
	}
	model.resourceCursor = 3
	model.selectedResource = &model.resources[1]
	model.focus = resourceFocus
	model.setResourceView(groupedResourceView)

	want := "+ core\n+ apps\n+ batch"
	if got := strings.Join(model.resourceLines(), "\n"); got != want {
		t.Fatalf("grouped resource rows =\n%s\nwant:\n%s", got, want)
	}
	if model.resourceCursor != 1 {
		t.Fatalf("resource cursor after grouping = %d, want the collapsed apps heading at row 1", model.resourceCursor)
	}
	if model.selectedResource == nil || model.selectedResource.Name != "pods" {
		t.Fatalf("switching views changed selected resource: %#v", model.selectedResource)
	}

	model.filters[resourceFocus] = "core"
	if got, want := strings.Join(model.resourceLines(), "\n"), "+ core"; got != want {
		t.Fatalf("filtering by the core API group =\n%s\nwant:\n%s", got, want)
	}
	model.filters[resourceFocus] = "apps"
	if got, want := strings.Join(model.resourceLines(), "\n"), "+ apps"; got != want {
		t.Fatalf("filtering by an API group =\n%s\nwant:\n%s", got, want)
	}
	model.filters[resourceFocus] = ""

	sendKey := func(key rune) {
		t.Helper()
		model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
	}
	model.resourceCursor = 1
	sendKey('h')
	if model.focus != resourceFocus || model.expandedResourceGroups["apps"] {
		t.Fatal("h on a collapsed group heading should keep resource focus and leave it collapsed")
	}
	sendKey('l')
	if !model.expandedResourceGroups["apps"] {
		t.Fatal("l on a collapsed group heading should expand it")
	}
	sendKey('h')
	if model.focus != resourceFocus || model.expandedResourceGroups["apps"] {
		t.Fatal("h on an expanded group heading should collapse it without changing focus")
	}
	sendKey('l')
	if !model.expandedResourceGroups["apps"] {
		t.Fatal("l on a collapsed group heading should expand it again")
	}
	want = "+ core\n- apps\n    deployments\n    replicasets\n+ batch"
	if got := strings.Join(model.resourceLines(), "\n"); got != want {
		t.Fatalf("expanded grouped resource rows =\n%s\nwant:\n%s", got, want)
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if model.expandedResourceGroups["apps"] {
		t.Fatal("Enter on the apps group should collapse its resource rows")
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !model.expandedResourceGroups["apps"] {
		t.Fatal("Enter on the collapsed apps group should expand its resource rows")
	}

	model.move(1)
	model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if model.selectedResource == nil || model.selectedResource.Name != "deployments" {
		t.Fatalf("selecting a grouped resource chose %#v, want deployments", model.selectedResource)
	}
	sendKey('h')
	if model.focus != workspaceFocus {
		t.Fatal("h on a resource row should keep its pane-navigation behavior")
	}
	model.focus = resourceFocus
	sendKey('l')
	if model.focus != instanceFocus {
		t.Fatal("l on a resource row should keep its pane-navigation behavior")
	}

	model.focus = resourceFocus
	model.resourceCursor = 1
	model.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	if model.expandedResourceGroups["apps"] {
		t.Fatal("Left on the apps group should collapse it")
	}
	model.setResourceView(flatResourceView)
	if model.resourceCursor != 0 {
		t.Fatalf("switching from a group heading to Flat should select its first resource, got cursor %d", model.resourceCursor)
	}
	model.setResourceView(groupedResourceView)
	if model.resourceCursor != 1 || model.expandedResourceGroups["apps"] {
		t.Fatalf("view switch should preserve the collapsed apps group: cursor=%d expanded=%v", model.resourceCursor, model.expandedResourceGroups["apps"])
	}
}

func TestResourceViewTabsAndScopedBracketShortcuts(t *testing.T) {
	model := testModel(t)
	model.width = 120
	model.height = 40
	model.resources = []kcp.Resource{
		{Name: "deployments", Group: "apps", Version: "v1", Kind: "Deployment"},
		{Name: "pods", Version: "v1", Kind: "Pod"},
	}
	sendKey := func(key rune) {
		t.Helper()
		model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
	}

	model.focus = resourceFocus
	flatHeader := ansi.Strip(strings.Split(model.renderPane(resourceFocus, "API resources", model.resourceLines(), 32, 6, false), "\n")[0])
	if !strings.Contains(flatHeader, "[Flat]| Group") {
		t.Fatalf("Flat resource tab should be visibly selected: %q", flatHeader)
	}
	model.setResourceView(groupedResourceView)
	groupHeader := ansi.Strip(strings.Split(model.renderPane(resourceFocus, "API resources", model.resourceLines(), 32, 6, false), "\n")[0])
	if !strings.Contains(groupHeader, " Flat |[Group]") {
		t.Fatalf("Group resource tab should be visibly selected: %q", groupHeader)
	}
	model.setResourceView(flatResourceView)

	sendKey(']')
	if model.resourceView != groupedResourceView {
		t.Fatal("] in the API resources pane should select Group")
	}
	sendKey('[')
	if model.resourceView != flatResourceView {
		t.Fatal("[ in the API resources pane should select Flat")
	}
	sendKey('4')
	if model.resourceView != groupedResourceView {
		t.Fatal("4 in the API resources pane should select Group")
	}
	sendKey('3')
	if model.resourceView != flatResourceView {
		t.Fatal("3 in the API resources pane should select Flat")
	}

	model.focus = detailFocus
	sendKey(']')
	if model.detailTab != 1 || model.resourceView != flatResourceView {
		t.Fatalf("] in Details should select YAML only, got detail tab %d and resource view %d", model.detailTab, model.resourceView)
	}
	sendKey('[')
	if model.detailTab != 0 || model.resourceView != flatResourceView {
		t.Fatalf("[ in Details should select JSON only, got detail tab %d and resource view %d", model.detailTab, model.resourceView)
	}

	model.focus = workspaceFocus
	sendKey(']')
	if model.detailTab != 0 || model.resourceView != flatResourceView {
		t.Fatal("bracket shortcuts outside Resources and Details should not change either view")
	}
	sendKey('2')
	if model.detailTab != 1 {
		t.Fatal("existing 1/2 detail shortcuts should remain available")
	}

	model.focus = detailFocus
	layout := model.layoutForView()
	lowerY := 1 + layout.workspaceHeight
	bounds, ok := model.paneBoundsAt(1, lowerY+1)
	if !ok || bounds.panel != resourceFocus {
		t.Fatal("could not locate the API resources pane")
	}
	clickTab := func(x int) {
		t.Helper()
		sendMouse(model, tea.MouseMsg{
			X:      x,
			Y:      bounds.y,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonLeft,
		})
	}
	fullTabStart := bounds.x + 1 + bounds.width - 2 - resourceTabFullTotalWidth
	clickTab(fullTabStart + resourceTabFullFlatWidth + 2)
	if model.resourceView != groupedResourceView || model.focus != resourceFocus {
		t.Fatal("clicking the Group tab should select Group and focus API resources")
	}
	clickTab(fullTabStart + 2)
	if model.resourceView != flatResourceView {
		t.Fatal("clicking the Flat tab should select Flat")
	}

	model.filters[resourceFocus] = "apps"
	compactTabStart := bounds.x + 1 + bounds.width - 2 - resourceTabCompactTotalWidth
	clickTab(compactTabStart + resourceTabCompactTabWidth + 2)
	if model.resourceView != groupedResourceView {
		t.Fatal("clicking the compact Group tab while filtering should select Group")
	}
	pane := model.renderPane(resourceFocus, "API resources", model.resourceLines(), minNarrowPaneWidth, 6, false)
	if got := lipgloss.Width(ansi.Strip(pane)); got != minNarrowPaneWidth {
		t.Fatalf("compact resource tabs rendered width %d at minimum pane size, want %d", got, minNarrowPaneWidth)
	}
	compactHeader := ansi.Strip(strings.Split(pane, "\n")[0])
	if !strings.Contains(compactHeader, "[G]") {
		t.Fatalf("compact tabs should indicate the active Group view: %q", compactHeader)
	}
}

func TestMouseGroupHeadersToggleAndResourceRowsSelect(t *testing.T) {
	model := testModel(t)
	model.width = 120
	model.height = 40
	model.resources = []kcp.Resource{
		{Name: "pods", Version: "v1", Kind: "Pod"},
		{Name: "deployments", Group: "apps", Version: "v1", Kind: "Deployment"},
		{Name: "replicasets", Group: "apps", Version: "v1", Kind: "ReplicaSet"},
	}
	model.resourceView = groupedResourceView
	model.resourceCursor = 2
	layout := model.layoutForView()
	lowerY := 1 + layout.workspaceHeight
	bounds, ok := model.paneBoundsAt(1, lowerY+1)
	if !ok || bounds.panel != resourceFocus {
		t.Fatal("could not locate the API resources pane")
	}
	clickRow := func(index int) {
		t.Helper()
		sendMouse(model, tea.MouseMsg{
			X:      bounds.x + 2,
			Y:      bounds.y + 1 + index,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonLeft,
		})
	}

	if got, want := strings.Join(model.resourceLines(), "\n"), "+ core\n+ apps"; got != want {
		t.Fatalf("groups should start collapsed, got %q want %q", got, want)
	}
	clickRow(1)
	if !model.expandedResourceGroups["apps"] {
		t.Fatal("clicking the collapsed apps heading should expand the group")
	}
	clickRow(1)
	if model.expandedResourceGroups["apps"] {
		t.Fatal("clicking the expanded apps heading should collapse the group")
	}
	clickRow(1)
	clickRow(2)
	if model.selectedResource == nil || model.selectedResource.Name != "deployments" {
		t.Fatalf("clicking a grouped resource selected %#v, want deployments", model.selectedResource)
	}
}

func TestResourceScrollingDoesNotScrollWorkspacePane(t *testing.T) {
	model := testModel(t)
	model.root.loaded = true
	model.root.expanded = true
	model.activeWorkspace = model.root.path
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("ws-%02d", i)
		model.root.children = append(model.root.children, &workspaceNode{
			name: name,
			path: model.root.path + ":" + name,
		})
		model.resources = append(model.resources, kcp.Resource{Name: fmt.Sprintf("resource-%02d", i)})
	}
	model.focus = resourceFocus
	model.resourceCursor = 10

	workspacePane := ansi.Strip(model.renderPane(workspaceFocus, "Workspaces", model.workspaceLines(), 32, 6, false))
	if !strings.Contains(workspacePane, "root *") || !strings.Contains(workspacePane, "ws-00") {
		t.Fatalf("resource cursor scrolled the workspace pane away from its top:\n%s", workspacePane)
	}

	resourcePane := ansi.Strip(model.renderPane(resourceFocus, "API resources", model.resourceLines(), 32, 6, true))
	if !strings.Contains(resourcePane, "resource-10") || strings.Contains(resourcePane, "resource-00") {
		t.Fatalf("focused resource pane did not follow its own cursor:\n%s", resourcePane)
	}
}
