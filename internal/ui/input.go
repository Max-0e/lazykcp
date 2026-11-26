package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleKey(key tea.KeyMsg) tea.Cmd {
	if m.filtering {
		return m.handleFilterKey(key)
	}
	if m.searching {
		return m.handleSearchKey(key)
	}
	switch key.Type {
	case tea.KeyCtrlC:
		return tea.Quit
	case tea.KeyCtrlD:
		return m.pageMove(1)
	case tea.KeyCtrlU:
		return m.pageMove(-1)
	case tea.KeyTab:
		m.changeFocus(1)
		return nil
	case tea.KeyShiftTab:
		m.changeFocus(-1)
		return nil
	case tea.KeyUp:
		return m.move(-1)
	case tea.KeyDown:
		return m.move(1)
	case tea.KeyLeft:
		if m.focus == workspaceFocus {
			m.collapseWorkspace()
		} else if m.focus == resourceFocus && m.collapseResourceGroup() {
			return nil
		} else {
			m.changeFocus(-1)
		}
		return nil
	case tea.KeyRight:
		if m.focus == workspaceFocus {
			return m.expandWorkspace()
		} else if m.focus == resourceFocus && m.expandResourceGroup() {
			return nil
		}
		m.changeFocus(1)
		return nil
	case tea.KeyEnter:
		return m.selectCurrent()
	case tea.KeyBackspace:
		if m.focus == workspaceFocus {
			m.collapseWorkspace()
		}
		return nil
	case tea.KeyEsc:
		if m.focus < detailFocus {
			m.filters[m.focus] = ""
		}
		return nil
	}

	switch key.String() {
	case "q":
		return tea.Quit
	case "j":
		return m.move(1)
	case "k":
		return m.move(-1)
	case "h":
		if m.focus == workspaceFocus {
			m.collapseWorkspace()
		} else if m.focus == resourceFocus && m.collapseResourceGroup() {
			return nil
		} else if m.focus > workspaceFocus {
			m.changeFocus(-1)
		}
	case "l":
		if m.focus == workspaceFocus {
			return m.expandWorkspace()
		} else if m.focus == resourceFocus && m.expandResourceGroup() {
			return nil
		} else {
			m.changeFocus(1)
		}
	case "/":
		if m.focus == detailFocus {
			m.searching = true
			m.searchText = ""
			m.findMatches()
		} else {
			m.filtering = true
			m.filterText = m.filters[m.focus]
		}
	case "n":
		m.moveMatch(1)
	case "N":
		m.moveMatch(-1)
	case "[":
		switch m.focus {
		case resourceFocus:
			m.setResourceView(flatResourceView)
		case detailFocus:
			m.setDetailTab(0)
		}
	case "]":
		switch m.focus {
		case resourceFocus:
			m.setResourceView(groupedResourceView)
		case detailFocus:
			m.setDetailTab(1)
		}
	case "1":
		m.setDetailTab(0)
	case "2":
		m.setDetailTab(1)
	case "3":
		if m.focus == resourceFocus {
			m.setResourceView(flatResourceView)
		}
	case "4":
		if m.focus == resourceFocus {
			m.setResourceView(groupedResourceView)
		}
	case "v":
		m.toggleDetailOnly()
	}
	return nil
}

func (m *Model) handleFilterKey(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyEnter:
		m.filters[m.focus] = m.filterText
		m.filtering = false
		m.filterText = ""
		m.resetCursor(m.focus)
	case tea.KeyEsc:
		m.filters[m.focus] = ""
		m.filtering = false
		m.filterText = ""
		m.resetCursor(m.focus)
	case tea.KeyBackspace:
		m.filterText = trimLastRune(m.filterText)
		m.updateFilter()
	default:
		if key.Type == tea.KeyRunes {
			m.filterText += string(key.Runes)
			m.updateFilter()
		}
	}
	return nil
}

func (m *Model) handleSearchKey(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyEnter:
		m.searching = false
		m.findMatches()
	case tea.KeyEsc:
		m.searching = false
		m.searchText = ""
		m.matches = nil
		m.scroll = 0
	case tea.KeyBackspace:
		m.searchText = trimLastRune(m.searchText)
		m.findMatches()
	default:
		if key.Type == tea.KeyRunes {
			m.searchText += string(key.Runes)
			m.findMatches()
		}
	}
	return nil
}

func (m *Model) move(delta int) tea.Cmd {
	switch m.focus {
	case workspaceFocus:
		items := m.filteredWorkspaces()
		if len(items) == 0 {
			return nil
		}
		index := m.workspaceIndex(items)
		index = clamp(index+delta, 0, len(items)-1)
		m.workspaceCursor = items[index].path
	case resourceFocus:
		rows := m.resourceRows()
		m.resourceCursor = clamp(m.resourceCursor+delta, 0, max(0, len(rows)-1))
	case instanceFocus:
		items := m.filteredInstances()
		m.instanceCursor = clamp(m.instanceCursor+delta, 0, max(0, len(items)-1))
	case detailFocus:
		m.scroll = max(0, m.scroll+delta)
	}
	return nil
}

func (m *Model) pageMove(direction int) tea.Cmd {
	step := max(1, m.paneVisibleRows(m.focus)/2)
	if m.focus == detailFocus {
		maxScroll := max(0, len(m.detailLines())-m.paneVisibleRows(detailFocus))
		m.scroll = clamp(m.scroll+direction*step, 0, maxScroll)
		return nil
	}
	return m.move(direction * step)
}

func (m *Model) changeFocus(delta int) {
	order := []focus{workspaceFocus, resourceFocus, instanceFocus, detailFocus}
	if m.detailOnly {
		order = []focus{workspaceFocus, detailFocus}
	}
	index := 0
	for i, panel := range order {
		if panel == m.focus {
			index = i
			break
		}
	}
	m.focus = order[(index+delta+len(order))%len(order)]
}

func (m *Model) toggleDetailOnly() {
	m.detailOnly = !m.detailOnly
	if m.detailOnly && (m.focus == resourceFocus || m.focus == instanceFocus) {
		m.focus = detailFocus
	}
}

func (m *Model) selectCurrent() tea.Cmd {
	switch m.focus {
	case workspaceFocus:
		node := m.findWorkspace(m.workspaceCursor)
		if node == nil {
			return nil
		}
		m.activeWorkspace = node.path
		m.selectedResource = nil
		m.selectedInstance = nil
		m.resources = nil
		m.instances = nil
		m.loading = "Discovering resources"
		cmds := []tea.Cmd{m.discoverResources(node.path)}
		if !node.loaded {
			node.expanded = true
			m.loading = "Loading workspace"
			cmds = append(cmds, m.loadWorkspaces(node.path))
		}
		return tea.Batch(cmds...)
	case resourceFocus:
		rows := m.resourceRows()
		if len(rows) == 0 {
			return nil
		}
		row := rows[clamp(m.resourceCursor, 0, len(rows)-1)]
		if row.groupHeader {
			m.activateResourceGroup(row.group)
			return nil
		}
		items := m.filteredResources()
		selected := items[row.resourceIndex]
		m.selectedResource = &selected
		m.instances = nil
		m.selectedInstance = nil
		m.loading = "Loading instances"
		return m.listInstances(m.activeWorkspace, selected)
	case instanceFocus:
		items := m.filteredInstances()
		if len(items) == 0 {
			return nil
		}
		selected := items[clamp(m.instanceCursor, 0, len(items)-1)]
		m.selectedInstance = &selected
		m.focus = detailFocus
		m.scroll = 0
		m.matches = nil
		return nil
	}
	return nil
}
