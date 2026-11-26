package ui

import (
	"strings"

	"github.com/Max-0e/lazykcp/internal/kcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func (m *Model) filteredWorkspaces() []*workspaceNode {
	var all []*workspaceNode
	var visit func(*workspaceNode)
	visit = func(node *workspaceNode) {
		all = append(all, node)
		if node.expanded {
			for _, child := range node.children {
				visit(child)
			}
		}
	}
	visit(m.root)
	filter := strings.ToLower(m.filters[workspaceFocus])
	if filter == "" {
		return all
	}
	filtered := make([]*workspaceNode, 0, len(all))
	for _, node := range all {
		if strings.Contains(strings.ToLower(node.name), filter) || strings.Contains(strings.ToLower(node.path), filter) {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

func (m *Model) filteredResources() []kcp.Resource {
	filter := strings.ToLower(m.filters[resourceFocus])
	if filter == "" {
		return m.resources
	}
	var filtered []kcp.Resource
	for _, resource := range m.resources {
		value := resource.Name + " " + resource.Kind + " " + resource.Group + " " + resourceGroupName(resource.Group)
		if strings.Contains(strings.ToLower(value), filter) {
			filtered = append(filtered, resource)
		}
	}
	return filtered
}

func (m *Model) filteredInstances() []unstructured.Unstructured {
	filter := strings.ToLower(m.filters[instanceFocus])
	if filter == "" {
		return m.instances
	}
	var filtered []unstructured.Unstructured
	for _, instance := range m.instances {
		value := instance.GetName() + " " + instance.GetNamespace()
		if strings.Contains(strings.ToLower(value), filter) {
			filtered = append(filtered, instance)
		}
	}
	return filtered
}

func (m *Model) workspaceIndex(items []*workspaceNode) int {
	for i, item := range items {
		if item.path == m.workspaceCursor {
			return i
		}
	}
	return 0
}

func (m *Model) resetCursor(panel focus) {
	switch panel {
	case workspaceFocus:
		items := m.filteredWorkspaces()
		if len(items) > 0 {
			m.workspaceCursor = items[0].path
		}
	case resourceFocus:
		m.resourceCursor = 0
	case instanceFocus:
		m.instanceCursor = 0
	}
}

func (m *Model) updateFilter() {
	m.filters[m.focus] = m.filterText
	m.resetCursor(m.focus)
}

func (m *Model) findMatches() {
	m.matches = nil
	m.matchIndex = 0
	m.scroll = 0
	if m.searchText == "" {
		return
	}
	needle := strings.ToLower(m.searchText)
	for line, text := range m.detailLines() {
		if strings.Contains(strings.ToLower(text), needle) {
			m.matches = append(m.matches, line)
		}
	}
	if len(m.matches) > 0 {
		m.scroll = m.matches[0]
	}
}

func (m *Model) moveMatch(delta int) {
	if len(m.matches) == 0 {
		return
	}
	m.matchIndex = (m.matchIndex + delta + len(m.matches)) % len(m.matches)
	m.scroll = m.matches[m.matchIndex]
}

func (m *Model) currentMatchLine() int {
	if len(m.matches) == 0 {
		return -1
	}
	return m.matches[m.matchIndex]
}

func (m *Model) setDetailTab(tab int) {
	m.detailTab = tab
	if m.searchText != "" {
		m.findMatches()
	}
}

func trimLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return value
	}
	return string(runes[:len(runes)-1])
}

func clip(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) > width {
		if width == 1 {
			return "…"
		}
		return string(runes[:width-1]) + "…"
	}
	return value
}

func clamp(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
