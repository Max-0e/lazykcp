package ui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type workspaceNode struct {
	name     string
	path     string
	children []*workspaceNode
	loaded   bool
	expanded bool
}

func (m *Model) expandWorkspace() tea.Cmd {
	node := m.findWorkspace(m.workspaceCursor)
	if node == nil {
		return nil
	}
	node.expanded = true
	if node.loaded {
		return nil
	}
	m.loading = "Loading workspaces"
	return m.loadWorkspaces(node.path)
}

func (m *Model) collapseWorkspace() {
	node := m.findWorkspace(m.workspaceCursor)
	if node == nil {
		return
	}
	if node.expanded {
		node.expanded = false
		return
	}
	if parent := m.parentWorkspace(node.path); parent != "" {
		m.workspaceCursor = parent
	}
}

func (m *Model) loadWorkspaces(parent string) tea.Cmd {
	return func() tea.Msg {
		items, err := m.client.ListWorkspaces(context.Background(), parent)
		return workspaceResult{parent: parent, items: items, err: err}
	}
}

func (m *Model) workspaceLines() []string {
	items := m.filteredWorkspaces()
	lines := make([]string, len(items))
	for i, node := range items {
		depth := strings.Count(node.path, ":") - strings.Count(m.root.path, ":")
		marker := m.workspaceMarker(node)
		prefix := strings.Repeat("  ", max(0, depth)) + marker + " "
		lines[i] = prefix + node.name
		if node.path == m.activeWorkspace {
			lines[i] += " *"
		}
	}
	return lines
}

func (m *Model) workspaceMarker(node *workspaceNode) string {
	if node.expanded {
		return "▼"
	}
	if !node.loaded || len(node.children) > 0 {
		return "▶"
	}
	return " "
}

func (m *Model) findWorkspace(path string) *workspaceNode {
	var find func(*workspaceNode) *workspaceNode
	find = func(node *workspaceNode) *workspaceNode {
		if node.path == path {
			return node
		}
		for _, child := range node.children {
			if found := find(child); found != nil {
				return found
			}
		}
		return nil
	}
	return find(m.root)
}

func (m *Model) parentWorkspace(path string) string {
	index := strings.LastIndex(path, ":")
	if index < 0 {
		return ""
	}
	parent := path[:index]
	if !strings.HasPrefix(parent, m.root.path) {
		return ""
	}
	return parent
}
