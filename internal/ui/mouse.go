package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleMouse(mouse tea.MouseMsg) tea.Cmd {
	if m.dragDivider != noDivider {
		switch mouse.Action {
		case tea.MouseActionRelease:
			m.dragDivider = noDivider
			return nil
		case tea.MouseActionMotion:
			if !m.filtering && !m.searching {
				m.resizeFrom(mouse.X, mouse.Y)
			}
			return nil
		default:
			return nil
		}
	}
	if m.filtering || m.searching {
		return nil
	}
	if mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft {
		if target, ok := m.dividerAt(mouse.X, mouse.Y); ok {
			m.beginResize(target, mouse.X, mouse.Y)
			return nil
		}
	}
	bounds, ok := m.paneBoundsAt(mouse.X, mouse.Y)
	if !ok {
		return nil
	}
	if mouse.Button == tea.MouseButtonWheelUp || mouse.Button == tea.MouseButtonWheelDown {
		m.focus = bounds.panel
		direction := 0
		switch mouse.Button {
		case tea.MouseButtonWheelUp:
			direction = -1
		case tea.MouseButtonWheelDown:
			direction = 1
		default:
			return nil
		}
		if bounds.panel == detailFocus {
			maxScroll := max(0, len(m.detailLines())-m.paneVisibleRows(detailFocus))
			m.scroll = clamp(m.scroll+direction*3, 0, maxScroll)
			return nil
		}
		return m.move(direction * 3)
	}
	if mouse.Action != tea.MouseActionPress || mouse.Button != tea.MouseButtonLeft {
		return nil
	}
	var lines []string
	var visibleRows, start int
	if bounds.panel != detailFocus {
		lines = m.linesForPanel(bounds.panel)
		visibleRows = m.paneVisibleRows(bounds.panel)
		start = m.paneRenderStart(bounds.panel, len(lines), visibleRows)
	}
	m.focus = bounds.panel
	if bounds.panel == detailFocus {
		if mouse.Y == bounds.y && m.selectedInstance != nil {
			if tab, ok := detailTabAt(bounds, mouse.X); ok {
				m.setDetailTab(tab)
			}
		}
		return nil
	}
	if bounds.panel == resourceFocus && mouse.Y == bounds.y {
		if tab, ok := m.resourceTabAt(bounds, mouse.X); ok {
			m.setResourceView(tab)
		}
		return nil
	}

	row := mouse.Y - bounds.y - 1
	index := start + row
	if row < 0 || row >= visibleRows || index < 0 || index >= len(lines) {
		return nil
	}

	switch bounds.panel {
	case workspaceFocus:
		items := m.filteredWorkspaces()
		node := items[index]
		depth := strings.Count(node.path, ":") - strings.Count(m.root.path, ":")
		markerColumn := 1 + max(0, depth)*2
		if mouse.X-bounds.x == markerColumn && m.workspaceMarker(node) != " " {
			m.workspaceCursor = node.path
			if node.expanded {
				node.expanded = false
				return nil
			}
			return m.expandWorkspace()
		}
		m.workspaceCursor = node.path
		return m.selectCurrent()
	case resourceFocus:
		rows := m.resourceRows()
		if index >= len(rows) {
			return nil
		}
		m.resourceCursor = index
		if rows[index].groupHeader {
			m.toggleResourceGroup(rows[index].group)
			return nil
		}
		return m.selectCurrent()
	case instanceFocus:
		m.instanceCursor = index
		cmd := m.selectCurrent()
		m.focus = instanceFocus
		return cmd
	}
	return nil
}
