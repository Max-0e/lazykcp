package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Max-0e/lazykcp/internal/kcp"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

type focus int

const (
	workspaceFocus focus = iota
	resourceFocus
	instanceFocus
	detailFocus
)

type viewLayout struct {
	workspaceHeight  int
	navigationWidth  int
	resourceHeight   int
	instanceHeight   int
	narrow           bool
	navigationHeight int
	resourceWidth    int
	instanceWidth    int
	detailWidth      int
	detailHeight     int
}

type paneBounds struct {
	panel  focus
	x      int
	y      int
	width  int
	height int
}

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

type paneSizes struct {
	workspaceHeight  int
	navigationWidth  int
	resourceHeight   int
	navigationHeight int
	resourceWidth    int
}

type divider int

const (
	noDivider divider = iota
	workspaceDivider
	navigationDivider
	resourceInstanceDivider
	narrowNavigationDivider
	narrowResourceInstanceDivider
)

const (
	detailTabWidth               = 6
	detailTabGroupWidth          = 13
	resourceTabFullTotalWidth    = 14
	resourceTabCompactTotalWidth = 7
	resourceTabFullMinInnerWidth = 22
	resourceTabFullFlatWidth     = 6
	resourceTabFullGroupWidth    = 7
	resourceTabCompactTabWidth   = 3
	minPaneExtent                = 3
	minNavigationWidth           = 20
	minNarrowPaneWidth           = 12
)

type workspaceNode struct {
	name     string
	path     string
	children []*workspaceNode
	loaded   bool
	expanded bool
}

type workspaceResult struct {
	parent string
	items  []*unstructured.Unstructured
	err    error
}

type resourceResult struct {
	workspace string
	items     []kcp.Resource
	err       error
}

type instanceResult struct {
	workspace string
	resource  kcp.Resource
	items     []unstructured.Unstructured
	err       error
}

type Model struct {
	client *kcp.Client
	root   *workspaceNode

	activeWorkspace  string
	workspaceCursor  string
	resourceCursor   int
	instanceCursor   int
	selectedResource *kcp.Resource
	selectedInstance *unstructured.Unstructured

	resources               []kcp.Resource
	instances               []unstructured.Unstructured
	resourceView            resourceView
	collapsedResourceGroups map[string]bool
	focus                   focus
	filters                 [3]string
	filtering               bool
	filterText              string
	searching               bool
	searchText              string
	matches                 []int
	matchIndex              int
	detailTab               int
	scroll                  int
	detailOnly              bool
	paneSizes               paneSizes
	dragDivider             divider
	dragOffset              int

	width   int
	height  int
	loading string
	status  string
}

func NewModel(client *kcp.Client) *Model {
	rootPath := client.RootPath()
	rootName := rootPath
	if index := strings.LastIndex(rootName, ":"); index >= 0 {
		rootName = rootName[index+1:]
	}
	root := &workspaceNode{name: rootName, path: rootPath, expanded: true}
	return &Model{
		client:          client,
		root:            root,
		activeWorkspace: rootPath,
		workspaceCursor: rootPath,
		focus:           workspaceFocus,
		loading:         "Loading workspaces",
	}
}

func (m *Model) Init() tea.Cmd {
	return m.selectCurrent()
}

func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.dragDivider = noDivider
		return m, nil
	case workspaceResult:
		node := m.findWorkspace(msg.parent)
		if node == nil {
			return m, nil
		}
		if msg.err != nil {
			m.status = msg.err.Error()
			m.loading = ""
			return m, nil
		}
		node.loaded = true
		for _, item := range msg.items {
			name := item.GetName()
			path := msg.parent + ":" + name
			found := false
			for _, child := range node.children {
				if child.path == path {
					found = true
					break
				}
			}
			if !found {
				node.children = append(node.children, &workspaceNode{name: name, path: path})
			}
		}
		m.loading = ""
		return m, nil
	case resourceResult:
		if msg.workspace != m.activeWorkspace {
			return m, nil
		}
		m.loading = ""
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.resources = msg.items
		m.resourceCursor = 0
		m.instances = nil
		m.selectedResource = nil
		m.selectedInstance = nil
		m.status = ""
		return m, nil
	case instanceResult:
		if msg.workspace != m.activeWorkspace || m.selectedResource == nil || *m.selectedResource != msg.resource {
			return m, nil
		}
		m.loading = ""
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.instances = msg.items
		m.instanceCursor = 0
		m.selectedInstance = nil
		m.status = ""
		return m, nil
	case tea.KeyMsg:
		return m, m.handleKey(msg)
	case tea.MouseMsg:
		return m, m.handleMouse(msg)
	}
	return m, nil
}

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
	var lowerPane string
	if m.detailOnly {
		lowerPane = m.renderPane(detailFocus, m.detailTitle(), m.detailLines(), layout.detailWidth, layout.detailHeight, m.focus == detailFocus)
	} else if !layout.narrow {
		navigationPane := lipgloss.JoinVertical(
			lipgloss.Left,
			m.renderPane(resourceFocus, "API resources", m.resourceLines(), layout.navigationWidth, layout.resourceHeight, m.focus == resourceFocus),
			m.renderPane(instanceFocus, "Instances", m.instanceLines(), layout.navigationWidth, layout.instanceHeight, m.focus == instanceFocus),
		)
		detailPane := m.renderPane(detailFocus, m.detailTitle(), m.detailLines(), layout.detailWidth, layout.detailHeight, m.focus == detailFocus)
		lowerPane = lipgloss.JoinHorizontal(lipgloss.Top, navigationPane, detailPane)
	} else {
		navigationPane := lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.renderPane(resourceFocus, "API resources", m.resourceLines(), layout.resourceWidth, layout.navigationHeight, m.focus == resourceFocus),
			m.renderPane(instanceFocus, "Instances", m.instanceLines(), layout.instanceWidth, layout.navigationHeight, m.focus == instanceFocus),
		)
		detailPane := m.renderPane(detailFocus, m.detailTitle(), m.detailLines(), layout.detailWidth, layout.detailHeight, m.focus == detailFocus)
		lowerPane = lipgloss.JoinVertical(lipgloss.Left, navigationPane, detailPane)
	}
	help := "j/k move ^D/^U page enter select / filter [] pane tab 3/4 view tab focus v nav mouse drag"
	if m.width >= 90 {
		help = "↑↓/jk move  Ctrl+D/U page  h/l focus  enter select  / filter  [] pane tab  3/4 resource view  v nav  mouse drag"
	}
	if m.width < 90 {
		if m.detailOnly {
			help += " (v show nav)"
		}
	}
	if m.filtering {
		help = "Filter: /" + m.filterText + "  enter close  esc clear"
	} else if m.searching {
		help = "Search: /" + m.searchText + "  enter close  esc cancel"
	} else if m.focus == detailFocus {
		help += "  n/N next/previous  1/2 JSON/YAML"
	}
	if lipgloss.Width(help) > m.width {
		help = clip(help, m.width)
	}
	return header + "\n" +
		lipgloss.JoinVertical(lipgloss.Left, workspacePane, lowerPane) + "\n" +
		lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(help)
}

func (m *Model) layoutForView() viewLayout {
	paneAreaHeight := m.height - 2
	workspaceHeight := m.paneSizes.workspaceHeight
	if workspaceHeight == 0 {
		workspaceHeight = clamp(paneAreaHeight/5, 5, 7)
	}
	workspaceHeight = clamp(workspaceHeight, 5, paneAreaHeight-7)
	lowerHeight := paneAreaHeight - workspaceHeight
	layout := viewLayout{
		workspaceHeight: workspaceHeight,
		detailWidth:     m.width,
		detailHeight:    lowerHeight,
	}
	if m.detailOnly {
		return layout
	}
	if m.width >= 80 {
		layout.navigationWidth = m.paneSizes.navigationWidth
		if layout.navigationWidth == 0 {
			layout.navigationWidth = clamp(m.width*28/100, minNavigationWidth, 36)
		}
		layout.navigationWidth = clamp(layout.navigationWidth, minNavigationWidth, m.width-minNavigationWidth)
		layout.resourceHeight = m.paneSizes.resourceHeight
		if layout.resourceHeight == 0 {
			layout.resourceHeight = lowerHeight / 2
		}
		layout.resourceHeight = clamp(layout.resourceHeight, minPaneExtent, lowerHeight-minPaneExtent)
		layout.instanceHeight = lowerHeight - layout.resourceHeight
		layout.detailWidth = m.width - layout.navigationWidth
		return layout
	}
	layout.narrow = true
	layout.navigationHeight = m.paneSizes.navigationHeight
	if layout.navigationHeight == 0 {
		layout.navigationHeight = min(max(4, lowerHeight/3), lowerHeight-minPaneExtent)
	}
	layout.navigationHeight = clamp(layout.navigationHeight, minPaneExtent, lowerHeight-minPaneExtent)
	layout.resourceWidth = m.paneSizes.resourceWidth
	if layout.resourceWidth == 0 {
		layout.resourceWidth = m.width / 2
	}
	layout.resourceWidth = clamp(layout.resourceWidth, minNarrowPaneWidth, m.width-minNarrowPaneWidth)
	layout.instanceWidth = m.width - layout.resourceWidth
	layout.detailHeight = lowerHeight - layout.navigationHeight
	return layout
}

func (m *Model) paneVisibleRows(panel focus) int {
	layout := m.layoutForView()
	var height int
	switch panel {
	case workspaceFocus:
		height = layout.workspaceHeight
	case resourceFocus:
		if layout.narrow {
			height = layout.navigationHeight
		} else {
			height = layout.resourceHeight
		}
	case instanceFocus:
		if layout.narrow {
			height = layout.navigationHeight
		} else {
			height = layout.instanceHeight
		}
	case detailFocus:
		height = layout.detailHeight
	}
	return max(1, height-2)
}

func (m *Model) paneBoundsAt(x, y int) (paneBounds, bool) {
	if m.width < 40 || m.height < 16 {
		return paneBounds{}, false
	}
	layout := m.layoutForView()
	workspace := paneBounds{panel: workspaceFocus, x: 0, y: 1, width: m.width, height: layout.workspaceHeight}
	if containsPoint(workspace, x, y) {
		return workspace, true
	}

	lowerY := workspace.y + workspace.height
	if m.detailOnly {
		detail := paneBounds{panel: detailFocus, x: 0, y: lowerY, width: m.width, height: layout.detailHeight}
		if containsPoint(detail, x, y) {
			return detail, true
		}
		return paneBounds{}, false
	}
	if !layout.narrow {
		if containsPoint(paneBounds{panel: resourceFocus, x: 0, y: lowerY, width: layout.navigationWidth, height: layout.resourceHeight}, x, y) {
			return paneBounds{panel: resourceFocus, x: 0, y: lowerY, width: layout.navigationWidth, height: layout.resourceHeight}, true
		}
		instanceY := lowerY + layout.resourceHeight
		if containsPoint(paneBounds{panel: instanceFocus, x: 0, y: instanceY, width: layout.navigationWidth, height: layout.instanceHeight}, x, y) {
			return paneBounds{panel: instanceFocus, x: 0, y: instanceY, width: layout.navigationWidth, height: layout.instanceHeight}, true
		}
		detail := paneBounds{panel: detailFocus, x: layout.navigationWidth, y: lowerY, width: layout.detailWidth, height: layout.detailHeight}
		if containsPoint(detail, x, y) {
			return detail, true
		}
		return paneBounds{}, false
	}

	if containsPoint(paneBounds{panel: resourceFocus, x: 0, y: lowerY, width: layout.resourceWidth, height: layout.navigationHeight}, x, y) {
		return paneBounds{panel: resourceFocus, x: 0, y: lowerY, width: layout.resourceWidth, height: layout.navigationHeight}, true
	}
	if containsPoint(paneBounds{panel: instanceFocus, x: layout.resourceWidth, y: lowerY, width: layout.instanceWidth, height: layout.navigationHeight}, x, y) {
		return paneBounds{panel: instanceFocus, x: layout.resourceWidth, y: lowerY, width: layout.instanceWidth, height: layout.navigationHeight}, true
	}
	detailY := lowerY + layout.navigationHeight
	detail := paneBounds{panel: detailFocus, x: 0, y: detailY, width: layout.detailWidth, height: layout.detailHeight}
	if containsPoint(detail, x, y) {
		return detail, true
	}
	return paneBounds{}, false
}

func containsPoint(bounds paneBounds, x, y int) bool {
	return x >= bounds.x && x < bounds.x+bounds.width &&
		y >= bounds.y && y < bounds.y+bounds.height
}

func (m *Model) dividerAt(x, y int) (divider, bool) {
	if m.width < 40 || m.height < 16 {
		return noDivider, false
	}
	layout := m.layoutForView()
	lowerY := 1 + layout.workspaceHeight
	if y == lowerY-1 {
		return workspaceDivider, true
	}
	if m.detailOnly {
		return noDivider, false
	}
	if !layout.narrow {
		if x == layout.navigationWidth-1 && y >= lowerY && y < lowerY+layout.detailHeight {
			return navigationDivider, true
		}
		if y == lowerY+layout.resourceHeight-1 && x >= 0 && x < layout.navigationWidth-1 {
			return resourceInstanceDivider, true
		}
		return noDivider, false
	}
	if y == lowerY+layout.navigationHeight-1 {
		return narrowNavigationDivider, true
	}
	if x == layout.resourceWidth-1 && y >= lowerY && y < lowerY+layout.navigationHeight-1 {
		return narrowResourceInstanceDivider, true
	}
	return noDivider, false
}

func (m *Model) beginResize(target divider, x, y int) {
	layout := m.layoutForView()
	lowerY := 1 + layout.workspaceHeight
	boundary := 0
	switch target {
	case workspaceDivider:
		boundary = lowerY
	case navigationDivider:
		boundary = layout.navigationWidth
	case resourceInstanceDivider:
		boundary = lowerY + layout.resourceHeight
	case narrowNavigationDivider:
		boundary = lowerY + layout.navigationHeight
	case narrowResourceInstanceDivider:
		boundary = layout.resourceWidth
	}
	if target == navigationDivider || target == narrowResourceInstanceDivider {
		m.dragOffset = x - boundary
	} else {
		m.dragOffset = y - boundary
	}
	m.dragDivider = target
}

func (m *Model) resizeFrom(x, y int) {
	layout := m.layoutForView()
	lowerY := 1 + layout.workspaceHeight
	switch m.dragDivider {
	case workspaceDivider:
		paneAreaHeight := m.height - 2
		m.paneSizes.workspaceHeight = clamp(y-m.dragOffset-1, 5, paneAreaHeight-7)
	case navigationDivider:
		m.paneSizes.navigationWidth = clamp(x-m.dragOffset, minNavigationWidth, m.width-minNavigationWidth)
	case resourceInstanceDivider:
		m.paneSizes.resourceHeight = clamp(y-m.dragOffset-lowerY, minPaneExtent, layout.detailHeight-minPaneExtent)
	case narrowNavigationDivider:
		lowerHeight := layout.navigationHeight + layout.detailHeight
		m.paneSizes.navigationHeight = clamp(y-m.dragOffset-lowerY, minPaneExtent, lowerHeight-minPaneExtent)
	case narrowResourceInstanceDivider:
		m.paneSizes.resourceWidth = clamp(x-m.dragOffset, minNarrowPaneWidth, m.width-minNarrowPaneWidth)
	}
}

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
		} else if m.focus > workspaceFocus {
			m.changeFocus(-1)
		}
	case "l":
		if m.focus == workspaceFocus {
			return m.expandWorkspace()
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
			m.toggleResourceGroup(row.group)
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
		return "-"
	}
	if !node.loaded || len(node.children) > 0 {
		return "+"
	}
	return " "
}

func (m *Model) resourceLines() []string {
	items := m.filteredResources()
	rows := m.resourceRows()
	lines := make([]string, len(rows))
	for i, row := range rows {
		if row.groupHeader {
			marker := "-"
			if m.collapsedResourceGroups[row.group] {
				marker = "+"
			}
			lines[i] = marker + " " + resourceGroupName(row.group)
			continue
		}
		resource := items[row.resourceIndex]
		if m.resourceView == groupedResourceView {
			lines[i] = "  " + resource.Name
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
		if m.collapsedResourceGroups[group] {
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
	if m.collapsedResourceGroups == nil {
		m.collapsedResourceGroups = make(map[string]bool)
	}
	m.collapsedResourceGroups[group] = !m.collapsedResourceGroups[group]
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
	if m.collapsedResourceGroups == nil {
		m.collapsedResourceGroups = make(map[string]bool)
	}
	m.collapsedResourceGroups[group] = true
	return true
}

func (m *Model) expandResourceGroup() bool {
	group, ok := m.focusedResourceGroup()
	if !ok {
		return false
	}
	delete(m.collapsedResourceGroups, group)
	return true
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

func (m *Model) detailTitle() string {
	tab := "JSON"
	if m.detailTab == 1 {
		tab = "YAML"
	}
	if m.selectedInstance == nil {
		return "Resource detail"
	}
	return fmt.Sprintf("%s · %s", m.selectedInstance.GetName(), tab)
}

func (m *Model) detailLines() []string {
	if m.selectedInstance == nil {
		return []string{"Select an instance to inspect"}
	}
	var data []byte
	var err error
	if m.detailTab == 0 {
		data, err = json.MarshalIndent(m.selectedInstance.Object, "", "  ")
	} else {
		var jsonData []byte
		jsonData, err = json.Marshal(m.selectedInstance.Object)
		if err == nil {
			data, err = yaml.JSONToYAML(jsonData)
		}
	}
	if err != nil {
		return []string{"Unable to serialize object: " + err.Error()}
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func (m *Model) renderPane(panel focus, title string, lines []string, width, height int, focused bool) string {
	innerWidth := max(1, width-2)
	innerHeight := max(1, height-2)
	showDetailTabs := panel == detailFocus && m.selectedInstance != nil
	showResourceTabs := panel == resourceFocus
	if showDetailTabs {
		title = m.selectedInstance.GetName()
	}
	query := m.queryForPanel(panel)
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
		titleWidth = max(0, innerWidth-detailTabGroupWidth)
		tabsWidth = detailTabGroupWidth
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
		top += m.renderDetailTabs(borderStyle)
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
		}
		line += strings.Repeat(" ", max(0, innerWidth-lipgloss.Width(line)))
		if panel == detailFocus && focused && index == m.currentMatchLine() {
			line = lipgloss.NewStyle().Background(lipgloss.Color("220")).Foreground(lipgloss.Color("0")).Render(line)
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

func (m *Model) renderDetailTabs(borderStyle lipgloss.Style) string {
	active := lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("205")).Bold(true)
	inactive := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	jsonTab := inactive
	yamlTab := inactive
	jsonLabel := " JSON "
	yamlLabel := " YAML "
	if m.detailTab == 0 {
		jsonTab = active
		jsonLabel = "[JSON]"
	} else {
		yamlTab = active
		yamlLabel = "[YAML]"
	}
	return jsonTab.Render(jsonLabel) +
		borderStyle.Render("|") +
		yamlTab.Render(yamlLabel)
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

func detailTabAt(bounds paneBounds, x int) (int, bool) {
	start := bounds.x + 1 + max(0, bounds.width-2-detailTabGroupWidth)
	offset := x - start
	switch {
	case offset >= 0 && offset < detailTabWidth:
		return 0, true
	case offset >= detailTabWidth+1 && offset < detailTabGroupWidth:
		return 1, true
	default:
		return 0, false
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
