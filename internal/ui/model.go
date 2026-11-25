package ui

import (
	"context"
	"encoding/json"
	"fmt"
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

	resources  []kcp.Resource
	instances  []unstructured.Unstructured
	focus      focus
	filters    [3]string
	filtering  bool
	filterText string
	searching  bool
	searchText string
	matches    []int
	matchIndex int
	detailTab  int
	scroll     int
	detailOnly bool

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
	return m.loadWorkspaces(m.root.path)
}

func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
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

	paneWidth := m.width
	paneAreaHeight := m.height - 2
	workspaceHeight := clamp(paneAreaHeight/5, 5, 7)
	if workspaceHeight > paneAreaHeight-7 {
		workspaceHeight = paneAreaHeight - 7
	}
	lowerHeight := paneAreaHeight - workspaceHeight
	workspacePane := m.renderPane("Workspaces", m.workspaceLines(), paneWidth, workspaceHeight, m.focus == workspaceFocus)
	var lowerPane string
	if m.detailOnly {
		lowerPane = m.renderPane(m.detailTitle(), m.detailLines(), paneWidth, lowerHeight, m.focus == detailFocus)
	} else if paneWidth >= 80 {
		navigationWidth := clamp(paneWidth*28/100, 20, 36)
		resourceHeight := lowerHeight / 2
		instanceHeight := lowerHeight - resourceHeight
		navigationPane := lipgloss.JoinVertical(
			lipgloss.Left,
			m.renderPane("API resources", m.resourceLines(), navigationWidth, resourceHeight, m.focus == resourceFocus),
			m.renderPane("Instances", m.instanceLines(), navigationWidth, instanceHeight, m.focus == instanceFocus),
		)
		detailPane := m.renderPane(m.detailTitle(), m.detailLines(), paneWidth-navigationWidth, lowerHeight, m.focus == detailFocus)
		lowerPane = lipgloss.JoinHorizontal(lipgloss.Top, navigationPane, detailPane)
	} else {
		navigationHeight := min(max(4, lowerHeight/3), lowerHeight-3)
		resourceWidth := paneWidth / 2
		navigationPane := lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.renderPane("API resources", m.resourceLines(), resourceWidth, navigationHeight, m.focus == resourceFocus),
			m.renderPane("Instances", m.instanceLines(), paneWidth-resourceWidth, navigationHeight, m.focus == instanceFocus),
		)
		detailPane := m.renderPane(m.detailTitle(), m.detailLines(), paneWidth, lowerHeight-navigationHeight, m.focus == detailFocus)
		lowerPane = lipgloss.JoinVertical(lipgloss.Left, navigationPane, detailPane)
	}
	help := "j/k:move tab:focus enter:select /:filter v:nav q:quit"
	if m.width >= 90 {
		help = "↑/k ↓/j move  ←/h →/l focus/expand  enter select  / filter/search  tab focus  "
		if m.detailOnly {
			help += "v show navigation  "
		} else {
			help += "v hide navigation  "
		}
		help += "q quit"
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
		help += "  n/N next/previous  [/] or 1/2 JSON/YAML"
	}
	if lipgloss.Width(help) > m.width {
		help = clip(help, m.width)
	}
	return header + "\n" +
		lipgloss.JoinVertical(lipgloss.Left, workspacePane, lowerPane) + "\n" +
		lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(help)
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
		} else {
			m.changeFocus(-1)
		}
		return nil
	case tea.KeyRight:
		if m.focus == workspaceFocus {
			return m.expandWorkspace()
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
		m.setDetailTab(0)
	case "]":
		m.setDetailTab(1)
	case "1":
		m.setDetailTab(0)
	case "2":
		m.setDetailTab(1)
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
		items := m.filteredResources()
		m.resourceCursor = clamp(m.resourceCursor+delta, 0, max(0, len(items)-1))
	case instanceFocus:
		items := m.filteredInstances()
		m.instanceCursor = clamp(m.instanceCursor+delta, 0, max(0, len(items)-1))
	case detailFocus:
		m.scroll = max(0, m.scroll+delta)
	}
	return nil
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
		items := m.filteredResources()
		if len(items) == 0 {
			return nil
		}
		selected := items[clamp(m.resourceCursor, 0, len(items)-1)]
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
		marker := " "
		if !node.loaded {
			marker = "+"
		} else if node.expanded {
			marker = "-"
		}
		prefix := strings.Repeat("  ", max(0, depth)) + marker + " "
		lines[i] = prefix + node.name
		if node.path == m.activeWorkspace {
			lines[i] += " *"
		}
	}
	return lines
}

func (m *Model) resourceLines() []string {
	items := m.filteredResources()
	lines := make([]string, len(items))
	for i, resource := range items {
		group := resource.Group
		if group == "" {
			group = "core"
		}
		lines[i] = resource.Name + " (" + group + ")"
	}
	return lines
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

func (m *Model) renderPane(title string, lines []string, width, height int, focused bool) string {
	innerWidth := max(1, width-2)
	innerHeight := max(1, height-3)
	titleStyle := lipgloss.NewStyle().Bold(true)
	if focused {
		titleStyle = titleStyle.Bold(true).Foreground(lipgloss.Color("205"))
	}
	if title == m.detailTitle() && m.selectedInstance != nil {
		title = "[" + title + "]"
	}
	rows := make([]string, 0, innerHeight)
	if len(lines) == 0 {
		lines = []string{" "}
	}
	selected := m.selectedLineIndex()
	start := 0
	if focused {
		if m.focus == detailFocus {
			start = clamp(m.scroll, 0, max(0, len(lines)-innerHeight))
		} else if selected >= innerHeight {
			start = selected - innerHeight + 1
		}
	}
	for row := 0; row < innerHeight; row++ {
		index := start + row
		if index >= len(lines) {
			rows = append(rows, "")
			continue
		}
		line := clip(lines[index], innerWidth)
		if focused && m.focus == detailFocus && index == m.currentMatchLine() {
			line = lipgloss.NewStyle().Background(lipgloss.Color("220")).Foreground(lipgloss.Color("0")).Render(line)
		} else if focused && index == selected {
			if m.focus == workspaceFocus {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Background(lipgloss.Color("235")).Bold(true).Render(line)
			} else {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true).Render("› " + clip(line, max(1, innerWidth-2)))
			}
		}
		rows = append(rows, line)
	}
	content := titleStyle.Render(title) + "\n" + strings.Join(rows, "\n")
	borderColor := lipgloss.Color("240")
	if focused {
		borderColor = lipgloss.Color("205")
	}
	return lipgloss.NewStyle().
		Width(innerWidth).
		Height(innerHeight + 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Render(content)
}

func (m *Model) selectedLineIndex() int {
	switch m.focus {
	case workspaceFocus:
		return m.workspaceIndex(m.filteredWorkspaces())
	case resourceFocus:
		return m.resourceCursor
	case instanceFocus:
		return m.instanceCursor
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
		value := resource.Name + " " + resource.Kind + " " + resource.Group
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
