package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestWorkspaceSelectionHighlightDoesNotShiftTreeRows(t *testing.T) {
	model := testModel(t)
	model.root.loaded = true
	model.root.expanded = true
	model.root.children = []*workspaceNode{
		{name: "team", path: "root:team", loaded: true, expanded: true},
	}
	model.workspaceCursor = "root:team"
	model.activeWorkspace = "root:team"
	model.focus = workspaceFocus

	selected := ansi.Strip(model.renderPane(workspaceFocus, "Workspaces", model.workspaceLines(), 32, 8, true))
	model.focus = resourceFocus
	unselected := ansi.Strip(model.renderPane(workspaceFocus, "Workspaces", model.workspaceLines(), 32, 8, false))

	if selected != unselected {
		t.Fatalf("workspace selection changed row text or alignment:\nselected:\n%s\nunselected:\n%s", selected, unselected)
	}
	if !strings.Contains(selected, "  ▼ team *") {
		t.Fatalf("selected workspace row lost its tree marker or label: %s", selected)
	}
}

func TestWorkspaceMarkerReflectsExpansionState(t *testing.T) {
	model := testModel(t)
	tests := []struct {
		name string
		node workspaceNode
		want string
	}{
		{name: "expanded", node: workspaceNode{expanded: true}, want: "▼"},
		{name: "not loaded", node: workspaceNode{}, want: "▶"},
		{name: "has children", node: workspaceNode{loaded: true, children: []*workspaceNode{{name: "child"}}}, want: "▶"},
		{name: "loaded leaf", node: workspaceNode{loaded: true}, want: " "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := model.workspaceMarker(&tt.node); got != tt.want {
				t.Fatalf("workspace marker = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMouseClickWorkspaceMarkerAndRow(t *testing.T) {
	model := testModel(t)
	model.width = 120
	model.height = 40
	model.root.loaded = true
	model.root.expanded = true
	child := &workspaceNode{
		name:   "team",
		path:   "root:team",
		loaded: true,
		children: []*workspaceNode{
			{name: "nested", path: "root:team:nested"},
		},
	}
	model.root.children = []*workspaceNode{child}
	markerClick := tea.MouseMsg{
		X:      3,
		Y:      1 + 1 + 1,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	}
	sendMouse(model, markerClick)
	if !child.expanded {
		t.Fatal("clicking a collapsed workspace marker should expand it")
	}
	if !strings.HasPrefix(model.workspaceLines()[1], "  ▼") {
		t.Fatalf("expanded workspace marker not rendered: %#v", model.workspaceLines())
	}
	sendMouse(model, markerClick)
	if child.expanded {
		t.Fatal("clicking an expanded workspace marker should collapse it")
	}
	if !strings.HasPrefix(model.workspaceLines()[1], "  ▶") {
		t.Fatalf("collapsed workspace marker not rendered: %#v", model.workspaceLines())
	}
	command := sendMouse(model, tea.MouseMsg{
		X:      6,
		Y:      markerClick.Y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if command == nil || model.activeWorkspace != child.path {
		t.Fatalf("workspace row click did not activate %q", child.path)
	}
}
