package ui

import (
	"context"
	"sort"

	"github.com/Max-0e/lazykcp/internal/kcp"
	tea "github.com/charmbracelet/bubbletea"
)

type resourceView int

const (
	flatResourceView resourceView = iota
	groupedResourceView
)

type resourceRow struct {
	group         string
	resourceIndex int
	groupHeader   bool
}

func (m *Model) discoverResources(workspace string) tea.Cmd {
	return func() tea.Msg {
		items, err := m.client.DiscoverResources(context.Background(), workspace)
		return resourceResult{workspace: workspace, items: items, err: err}
	}
}

func (m *Model) listInstances(workspace string, resource kcp.Resource) tea.Cmd {
	return func() tea.Msg {
		items, err := m.client.ListInstances(context.Background(), workspace, resource)
		return instanceResult{workspace: workspace, resource: resource, items: items, err: err}
	}
}

func (m *Model) resourceLines() []string {
	items := m.filteredResources()
	rows := m.resourceRows()
	lines := make([]string, len(rows))
	for i, row := range rows {
		if row.groupHeader {
			marker := "▶"
			if m.expandedResourceGroups[row.group] {
				marker = "▼"
			}
			lines[i] = marker + " " + resourceGroupName(row.group)
			continue
		}
		resource := items[row.resourceIndex]
		if m.resourceView == groupedResourceView {
			lines[i] = "    " + resource.Name
			continue
		}
		lines[i] = resource.Name + " (" + resourceGroupName(resource.Group) + ")"
	}
	return lines
}

func resourceGroupName(group string) string {
	if group == "" {
		return "core"
	}
	return group
}

func (m *Model) resourceRows() []resourceRow {
	items := m.filteredResources()
	if m.resourceView == flatResourceView {
		rows := make([]resourceRow, len(items))
		for i, resource := range items {
			rows[i] = resourceRow{group: resource.Group, resourceIndex: i}
		}
		return rows
	}
	groups := make(map[string][]int)
	groupNames := make([]string, 0)
	for i, resource := range items {
		group := resource.Group
		if _, exists := groups[group]; !exists {
			groupNames = append(groupNames, group)
		}
		groups[group] = append(groups[group], i)
	}
	sort.Strings(groupNames)

	var rows []resourceRow
	for _, group := range groupNames {
		rows = append(rows, resourceRow{group: group, groupHeader: true})
		if !m.expandedResourceGroups[group] {
			continue
		}
		for _, resourceIndex := range groups[group] {
			rows = append(rows, resourceRow{group: group, resourceIndex: resourceIndex})
		}
	}
	return rows
}

func (m *Model) setResourceView(view resourceView) {
	if view != flatResourceView && view != groupedResourceView || view == m.resourceView {
		return
	}

	items := m.filteredResources()
	var current resourceRow
	hasCurrent := false
	rows := m.resourceRows()
	if len(rows) > 0 {
		cursor := clamp(m.resourceCursor, 0, len(rows)-1)
		current = rows[cursor]
		hasCurrent = true
	}
	var currentResource kcp.Resource
	hasCurrentResource := hasCurrent && !current.groupHeader && current.resourceIndex >= 0 && current.resourceIndex < len(items)
	if hasCurrentResource {
		currentResource = items[current.resourceIndex]
	}

	m.resourceView = view
	newRows := m.resourceRows()
	target := -1
	if hasCurrentResource {
		groupHeader := -1
		newItems := m.filteredResources()
		for i, row := range newRows {
			if row.groupHeader {
				if row.group == currentResource.Group {
					groupHeader = i
				}
				continue
			}
			resource := newItems[row.resourceIndex]
			if resource.Group == currentResource.Group &&
				resource.Version == currentResource.Version &&
				resource.Name == currentResource.Name {
				target = i
				break
			}
		}
		if target < 0 {
			target = groupHeader
		}
	} else if hasCurrent && current.groupHeader {
		if view == groupedResourceView {
			for i, row := range newRows {
				if row.groupHeader && row.group == current.group {
					target = i
					break
				}
			}
		} else {
			newItems := m.filteredResources()
			for i, resource := range newItems {
				if resource.Group == current.group {
					target = i
					break
				}
			}
		}
	}
	if target < 0 {
		target = 0
	}
	m.resourceCursor = clamp(target, 0, max(0, len(newRows)-1))
}

func (m *Model) toggleResourceGroup(group string) {
	if m.expandedResourceGroups[group] {
		delete(m.expandedResourceGroups, group)
		return
	}
	if m.expandedResourceGroups == nil {
		m.expandedResourceGroups = make(map[string]bool)
	}
	m.expandedResourceGroups[group] = true
}

func (m *Model) activateResourceGroup(group string) {
	m.selectedResource = nil
	m.selectedInstance = nil
	m.instances = nil
	m.instanceCursor = 0
	m.searching = false
	m.searchText = ""
	m.matches = nil
	m.matchIndex = 0
	m.scroll = 0
	m.status = ""
	if m.loading == "Loading instances" {
		m.loading = ""
	}
	m.toggleResourceGroup(group)
}

func (m *Model) focusedResourceGroup() (string, bool) {
	if m.resourceView != groupedResourceView {
		return "", false
	}
	rows := m.resourceRows()
	if len(rows) == 0 {
		return "", false
	}
	row := rows[clamp(m.resourceCursor, 0, len(rows)-1)]
	if !row.groupHeader {
		return "", false
	}
	return row.group, true
}

func (m *Model) collapseResourceGroup() bool {
	group, ok := m.focusedResourceGroup()
	if !ok {
		return false
	}
	delete(m.expandedResourceGroups, group)
	return true
}

func (m *Model) expandResourceGroup() bool {
	group, ok := m.focusedResourceGroup()
	if !ok {
		return false
	}
	if m.expandedResourceGroups == nil {
		m.expandedResourceGroups = make(map[string]bool)
	}
	m.expandedResourceGroups[group] = true
	return true
}
