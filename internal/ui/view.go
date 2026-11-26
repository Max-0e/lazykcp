package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	detailTabsFullWidth          = 24
	detailTabsCompactWidth       = 11
	detailTabsFullMinInnerWidth  = 32
	detailOverviewTabFullWidth   = 10
	detailFormatTabFullWidth     = 6
	detailTabCompactWidth        = 3
	resourceTabFullTotalWidth    = 14
	resourceTabCompactTotalWidth = 7
	resourceTabFullMinInnerWidth = 22
	resourceTabFullFlatWidth     = 6
	resourceTabFullGroupWidth    = 7
	resourceTabCompactTabWidth   = 3
)

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Starting lazykcp..."
	}
	if m.width < 40 || m.height < 16 {
		return clip("Terminal too small; resize to at least 40x16.", m.width)
	}
	title := "lazykcp"
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render(title)
	remainingHeaderWidth := m.width - lipgloss.Width(title)
	appendHeader := func(text string, style lipgloss.Style) {
		if remainingHeaderWidth <= 0 {
			return
		}
		text = clip(text, remainingHeaderWidth)
		header += style.Render(text)
		remainingHeaderWidth -= lipgloss.Width(text)
	}
	appendHeader("  workspace: "+m.activeWorkspace, lipgloss.NewStyle())
	if m.loading != "" {
		appendHeader("  "+m.loading+"…", lipgloss.NewStyle().Foreground(lipgloss.Color("220")))
	}
	if m.status != "" {
		appendHeader("  "+m.status, lipgloss.NewStyle().Foreground(lipgloss.Color("196")))
	}

	layout := m.layoutForView()
	workspacePane := m.renderPane(workspaceFocus, "Workspaces", m.workspaceLines(), m.width, layout.workspaceHeight, m.focus == workspaceFocus)
	detailPane := func() string {
		title := m.detailTitle()
		if m.terminal != nil {
			title = "Terminal · " + m.terminal.workspace
		}
		return m.renderPane(detailFocus, title, m.detailLines(), layout.detailWidth, layout.detailHeight, m.focus == detailFocus)
	}
	var lowerPane string
	if m.detailOnly {
		lowerPane = detailPane()
	} else if !layout.narrow {
		navigationPane := lipgloss.JoinVertical(
			lipgloss.Left,
			m.renderPane(resourceFocus, "API resources", m.resourceLines(), layout.navigationWidth, layout.resourceHeight, m.focus == resourceFocus),
			m.renderPane(instanceFocus, "Instances", m.instanceLines(), layout.navigationWidth, layout.instanceHeight, m.focus == instanceFocus),
		)
		lowerPane = lipgloss.JoinHorizontal(lipgloss.Top, navigationPane, detailPane())
	} else {
		navigationPane := lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.renderPane(resourceFocus, "API resources", m.resourceLines(), layout.resourceWidth, layout.navigationHeight, m.focus == resourceFocus),
			m.renderPane(instanceFocus, "Instances", m.instanceLines(), layout.instanceWidth, layout.navigationHeight, m.focus == instanceFocus),
		)
		lowerPane = lipgloss.JoinVertical(lipgloss.Left, navigationPane, detailPane())
	}
	help := "j/k move ^D/^U page enter select / filter [] pane tab 3/4 view tab h/l group/focus t terminal v nav mouse drag"
	if m.width >= 90 {
		help = "↑↓/jk move  Ctrl+D/U page  h/l focus/group  enter select  / filter  [] pane tab  3/4 resource view  t terminal  v nav  mouse drag"
	}
	if m.width < 90 {
		if m.detailOnly {
			help += " (v show nav)"
		}
	}
	if m.terminal != nil {
		help = "Terminal focused · Ctrl+D exits to browsing"
		if m.terminal.failure != "" {
			help = m.terminal.failure
		}
	} else if m.filtering {
		help = "Filter: /" + m.filterText + "  enter close  esc clear"
	} else if m.searching {
		help = "Search: /" + m.searchText + "  enter close  esc cancel"
	} else if m.focus == detailFocus {
		help += "  n/N next/previous  o Overview  1/2 JSON/YAML"
	}
	if lipgloss.Width(help) > m.width {
		help = clip(help, m.width)
	}
	return header + "\n" +
		lipgloss.JoinVertical(lipgloss.Left, workspacePane, lowerPane) + "\n" +
		lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(help)
}

func (m *Model) linesForPanel(panel focus) []string {
	switch panel {
	case workspaceFocus:
		return m.workspaceLines()
	case resourceFocus:
		return m.resourceLines()
	case instanceFocus:
		return m.instanceLines()
	case detailFocus:
		return m.detailLines()
	default:
		return nil
	}
}

func (m *Model) paneRenderStart(panel focus, lineCount, visibleRows int) int {
	if panel == detailFocus {
		return clamp(m.scroll, 0, max(0, lineCount-visibleRows))
	}
	selected := m.selectedLineIndex(panel)
	if selected >= visibleRows {
		return selected - visibleRows + 1
	}
	return 0
}

func (m *Model) instanceLines() []string {
	items := m.filteredInstances()
	lines := make([]string, len(items))
	for i, instance := range items {
		name := instance.GetName()
		if namespace := instance.GetNamespace(); namespace != "" {
			name = namespace + "/" + name
		}
		lines[i] = name
	}
	return lines
}

func (m *Model) renderPane(panel focus, title string, lines []string, width, height int, focused bool) string {
	innerWidth := max(1, width-2)
	innerHeight := max(1, height-2)
	terminalPane := panel == detailFocus && m.terminal != nil
	if terminalPane {
		lines = m.terminalLines()
	}
	showDetailTabs := panel == detailFocus && !terminalPane && m.selectedInstance != nil
	showResourceTabs := panel == resourceFocus
	if showDetailTabs {
		title = m.selectedInstance.GetName()
	}
	query := m.queryForPanel(panel)
	if terminalPane {
		query = ""
	}
	if len(lines) == 0 {
		switch panel {
		case resourceFocus:
			lines = []string{"No API resources"}
		case instanceFocus:
			switch {
			case m.selectedResource == nil:
				lines = []string{"Select a resource"}
			case m.loading == "Loading instances":
				lines = []string{"Loading instances"}
			default:
				lines = []string{"No instances"}
			}
		default:
			lines = []string{" "}
		}
	}
	selected := m.selectedLineIndex(panel)
	start := m.paneRenderStart(panel, len(lines), innerHeight)
	if terminalPane {
		selected = -1
		start = 0
	}
	thumbStart, thumbHeight, scrollable := scrollThumb(start, innerHeight, len(lines))
	rows := make([]string, 0, innerHeight)
	borderColor := lipgloss.Color("240")
	titleColor := lipgloss.Color("252")
	if focused {
		borderColor = lipgloss.Color("205")
		titleColor = lipgloss.Color("205")
	}
	border := lipgloss.RoundedBorder()
	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	titleStyle := lipgloss.NewStyle().Foreground(titleColor).Bold(true)
	titleWidth := innerWidth
	tabsWidth := 0
	if showDetailTabs {
		tabsWidth, _, _, _, _ = detailTabsGeometry(innerWidth)
		titleWidth = max(0, innerWidth-tabsWidth)
	} else if showResourceTabs {
		tabsWidth, _, _ = resourceTabGeometry(innerWidth, query != "")
		titleWidth = max(0, innerWidth-tabsWidth)
	}
	label := renderPaneTitle(title, query, titleWidth)
	topFill := max(0, innerWidth-lipgloss.Width(label)-tabsWidth)
	top := borderStyle.Render(border.TopLeft) +
		titleStyle.Render(label) +
		borderStyle.Render(strings.Repeat(border.Top, topFill))
	if showDetailTabs {
		top += m.renderDetailTabs(borderStyle, innerWidth)
	} else if showResourceTabs {
		top += m.renderResourceTabs(borderStyle, innerWidth, query != "")
	}
	top += borderStyle.Render(border.TopRight)
	for row := 0; row < innerHeight; row++ {
		index := start + row
		line := ""
		if index >= len(lines) {
			line = ""
		} else {
			line = clip(lines[index], innerWidth)
			if terminalPane {
				line = ansi.Truncate(lines[index], innerWidth, "")
			}
		}
		line += strings.Repeat(" ", max(0, innerWidth-lipgloss.Width(line)))
		if !terminalPane && panel == detailFocus && focused && index == m.currentMatchLine() {
			line = lipgloss.NewStyle().Background(lipgloss.Color("220")).Foreground(lipgloss.Color("0")).Render(line)
		} else if !terminalPane && panel == detailFocus && m.detailView == overviewDetailView && index < len(lines) && overviewSectionHeading(lines[index]) {
			line = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true).Render(line)
		} else if panel != detailFocus && index == selected {
			if focused {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Background(lipgloss.Color("235")).Bold(true).Render(line)
			} else {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("238")).Bold(true).Render(line)
			}
		}
		rightBorder := border.Right
		rightStyle := borderStyle
		if scrollable {
			rightBorder = "┆"
			if row >= thumbStart && row < thumbStart+thumbHeight {
				rightBorder = "█"
				rightStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
			}
		}
		rows = append(rows, borderStyle.Render(border.Left)+line+rightStyle.Render(rightBorder))
	}
	bottom := borderStyle.Render(border.BottomLeft + strings.Repeat(border.Bottom, innerWidth) + border.BottomRight)
	return top + "\n" + strings.Join(rows, "\n") + "\n" + bottom
}

func (m *Model) queryForPanel(panel focus) string {
	switch panel {
	case workspaceFocus, resourceFocus, instanceFocus:
		return m.filters[panel]
	case detailFocus:
		return m.searchText
	default:
		return ""
	}
}

func renderPaneTitle(title, query string, width int) string {
	if query == "" {
		return clip(" "+title+" ", width)
	}
	queryLabel := "(/" + query + ")"
	queryWidth := lipgloss.Width(queryLabel)
	if queryWidth+3 > width {
		return clip(queryLabel, width)
	}
	titleWidth := width - queryWidth - 3
	if titleWidth == 0 {
		return queryLabel
	}
	return " " + clip(title, titleWidth) + " " + queryLabel + " "
}

func (m *Model) renderDetailTabs(borderStyle lipgloss.Style, innerWidth int) string {
	active := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("205")).Bold(true)
	inactive := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	_, overviewWidth, jsonWidth, yamlWidth, compact := detailTabsGeometry(innerWidth)
	tabs := []struct {
		view  detailView
		style lipgloss.Style
		label string
	}{
		{view: overviewDetailView, style: inactive, label: " Overview "},
		{view: jsonDetailView, style: inactive, label: " JSON "},
		{view: yamlDetailView, style: inactive, label: " YAML "},
	}
	widths := []int{overviewWidth, jsonWidth, yamlWidth}
	if compact {
		for i, label := range []string{"O", "J", "Y"} {
			tabs[i].label = " " + label + " "
			if tabs[i].view == m.detailView {
				tabs[i].label = "[" + label + "]"
			}
		}
	} else {
		for i, tab := range tabs {
			if tab.view == m.detailView {
				tabs[i].label = "[" + strings.TrimSpace(tab.label) + "]"
			}
		}
	}
	var rendered strings.Builder
	for i := range tabs {
		if i > 0 {
			rendered.WriteString(borderStyle.Render("|"))
		}
		if tabs[i].view == m.detailView {
			tabs[i].style = active
		}
		if lipgloss.Width(tabs[i].label) != widths[i] {
			tabs[i].label = clip(tabs[i].label, widths[i])
		}
		rendered.WriteString(tabs[i].style.Render(tabs[i].label))
	}
	return rendered.String()
}

func (m *Model) renderResourceTabs(borderStyle lipgloss.Style, innerWidth int, compact bool) string {
	active := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("205")).Bold(true)
	inactive := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	flatTab := inactive
	groupTab := inactive
	flatLabel := " Flat "
	groupLabel := " Group "
	if !compact && innerWidth >= resourceTabFullMinInnerWidth {
		if m.resourceView == flatResourceView {
			flatTab = active
			flatLabel = "[Flat]"
		} else {
			groupTab = active
			groupLabel = "[Group]"
		}
	} else {
		flatLabel = " F "
		groupLabel = " G "
		if m.resourceView == flatResourceView {
			flatTab = active
			flatLabel = "[F]"
		} else {
			groupTab = active
			groupLabel = "[G]"
		}
	}
	return flatTab.Render(flatLabel) +
		borderStyle.Render("|") +
		groupTab.Render(groupLabel)
}

func detailTabsGeometry(innerWidth int) (totalWidth, overviewWidth, jsonWidth, yamlWidth int, compact bool) {
	if innerWidth >= detailTabsFullMinInnerWidth {
		return detailTabsFullWidth, detailOverviewTabFullWidth, detailFormatTabFullWidth, detailFormatTabFullWidth, false
	}
	return detailTabsCompactWidth, detailTabCompactWidth, detailTabCompactWidth, detailTabCompactWidth, true
}

func detailTabAt(bounds paneBounds, x int) (detailView, bool) {
	innerWidth := max(1, bounds.width-2)
	totalWidth, overviewWidth, jsonWidth, yamlWidth, _ := detailTabsGeometry(innerWidth)
	start := bounds.x + 1 + innerWidth - totalWidth
	offset := x - start
	switch {
	case offset >= 0 && offset < overviewWidth:
		return overviewDetailView, true
	case offset >= overviewWidth+1 && offset < overviewWidth+1+jsonWidth:
		return jsonDetailView, true
	case offset >= overviewWidth+jsonWidth+2 && offset < overviewWidth+jsonWidth+2+yamlWidth:
		return yamlDetailView, true
	default:
		return overviewDetailView, false
	}
}

func resourceTabGeometry(innerWidth int, compact bool) (totalWidth, flatWidth, groupWidth int) {
	if !compact && innerWidth >= resourceTabFullMinInnerWidth {
		return resourceTabFullTotalWidth, resourceTabFullFlatWidth, resourceTabFullGroupWidth
	}
	return resourceTabCompactTotalWidth, resourceTabCompactTabWidth, resourceTabCompactTabWidth
}

func (m *Model) resourceTabAt(bounds paneBounds, x int) (resourceView, bool) {
	innerWidth := max(1, bounds.width-2)
	totalWidth, flatWidth, groupWidth := resourceTabGeometry(innerWidth, m.queryForPanel(resourceFocus) != "")
	start := bounds.x + 1 + innerWidth - totalWidth
	offset := x - start
	switch {
	case offset >= 0 && offset < flatWidth:
		return flatResourceView, true
	case offset >= flatWidth+1 && offset < flatWidth+1+groupWidth:
		return groupedResourceView, true
	default:
		return flatResourceView, false
	}
}

func scrollThumb(start, visibleRows, lineCount int) (top, height int, scrollable bool) {
	if visibleRows <= 0 || lineCount <= visibleRows {
		return 0, 0, false
	}
	height = max(1, visibleRows*visibleRows/lineCount)
	height = min(height, visibleRows)
	maxStart := lineCount - visibleRows
	start = clamp(start, 0, maxStart)
	top = start * (visibleRows - height) / maxStart
	return top, height, true
}

func (m *Model) selectedLineIndex(panel focus) int {
	switch panel {
	case workspaceFocus:
		items := m.filteredWorkspaces()
		for i, item := range items {
			if item.path == m.workspaceCursor {
				return i
			}
		}
		return -1
	case resourceFocus:
		rows := m.resourceRows()
		if len(rows) == 0 {
			return -1
		}
		return clamp(m.resourceCursor, 0, len(rows)-1)
	case instanceFocus:
		items := m.filteredInstances()
		if len(items) == 0 {
			return -1
		}
		return clamp(m.instanceCursor, 0, len(items)-1)
	default:
		return -1
	}
}
