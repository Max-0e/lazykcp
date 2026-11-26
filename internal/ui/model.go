package ui

import (
	"strings"

	"github.com/Max-0e/lazykcp/internal/kcp"
	tea "github.com/charmbracelet/bubbletea"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type focus int

const (
	workspaceFocus focus = iota
	resourceFocus
	instanceFocus
	detailFocus
)

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

	resources              []kcp.Resource
	instances              []unstructured.Unstructured
	resourceView           resourceView
	expandedResourceGroups map[string]bool
	focus                  focus
	filters                [3]string
	filtering              bool
	filterText             string
	searching              bool
	searchText             string
	matches                []int
	matchIndex             int
	detailView             detailView
	scroll                 int
	detailOnly             bool
	paneSizes              paneSizes
	dragDivider            divider
	dragOffset             int
	terminal               *terminalSession
	terminalSequence       uint64
	terminalReturnFocus    focus

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
		detailView:      overviewDetailView,
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
		if m.terminal != nil {
			m.resizeTerminal()
		}
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
	case terminalOutputMsg:
		return m, m.handleTerminalOutput(msg)
	case terminalExitMsg:
		return m, m.handleTerminalExit(msg)
	case terminalInputDoneMsg:
		return m, m.handleTerminalInputDone(msg)
	case tea.KeyMsg:
		return m, m.handleKey(msg)
	case tea.MouseMsg:
		if m.terminal != nil {
			m.handleTerminalMouse(msg)
			return m, nil
		}
		return m, m.handleMouse(msg)
	}
	return m, nil
}
