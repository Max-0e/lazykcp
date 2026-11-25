package ui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Max-0e/lazykcp/internal/kcp"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func testModel(t *testing.T) *Model {
	t.Helper()
	config := `apiVersion: v1
kind: Config
clusters:
- name: kcp
  cluster:
    server: https://kcp.example/clusters/root
users:
- name: admin
  user: {}
contexts:
- name: admin@kcp
  context:
    cluster: kcp
    user: admin
current-context: admin@kcp
`
	path := t.TempDir() + "/config"
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	client, err := kcp.NewClient(path, false)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	return NewModel(client)
}

func sendMouse(model *Model, message tea.MouseMsg) tea.Cmd {
	_, command := model.Update(message)
	return command
}

func TestSidebarFiltersAndDetailRepresentations(t *testing.T) {
	model := testModel(t)
	model.root.children = []*workspaceNode{
		{name: "team-a", path: "root:team-a"},
		{name: "team-b", path: "root:team-b"},
	}
	model.filters[workspaceFocus] = "team-b"
	if got := model.filteredWorkspaces(); len(got) != 1 || got[0].name != "team-b" {
		t.Fatalf("filtered workspaces = %#v, want only team-b", got)
	}

	model.resources = []kcp.Resource{
		{Name: "pods", Kind: "Pod", Group: ""},
		{Name: "deployments", Kind: "Deployment", Group: "apps"},
	}
	model.filters[resourceFocus] = "apps"
	if got := model.filteredResources(); len(got) != 1 || got[0].Name != "deployments" {
		t.Fatalf("filtered resources = %#v, want deployments", got)
	}

	model.selectedInstance = &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]interface{}{"name": "settings"},
		"data":       map[string]interface{}{"theme": "dark"},
	}}
	model.detailTab = 0
	if got := strings.Join(model.detailLines(), "\n"); !strings.Contains(got, `"theme": "dark"`) {
		t.Fatalf("JSON detail missing instance data: %s", got)
	}
	model.searchText = "dark"
	model.findMatches()
	if len(model.matches) != 1 {
		t.Fatalf("JSON search matches = %v, want one match", model.matches)
	}
	model.setDetailTab(1)
	if got := strings.Join(model.detailLines(), "\n"); !strings.Contains(got, "theme: dark") {
		t.Fatalf("YAML detail missing instance data: %s", got)
	}
	if len(model.matches) != 1 {
		t.Fatalf("YAML search matches = %v, want one refreshed match", model.matches)
	}
}

func TestSidebarSlashFilterInput(t *testing.T) {
	model := testModel(t)
	model.resources = []kcp.Resource{
		{Name: "pods", Kind: "Pod"},
		{Name: "deployments", Kind: "Deployment", Group: "apps"},
	}
	model.focus = resourceFocus
	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if got := model.filteredResources(); len(got) != 0 {
		t.Fatalf("resources were not filtered before Enter: %#v", got)
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := model.filteredResources(); len(got) != 2 {
		t.Fatalf("backspace did not update results live: %#v", got)
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if model.filtering {
		t.Fatal("Enter should close the filter input")
	}
	if model.filters[resourceFocus] != "" {
		t.Fatalf("resource filter = %q, want an empty filter", model.filters[resourceFocus])
	}
}

func TestDetailSearchUpdatesMatchesBeforeEnter(t *testing.T) {
	model := testModel(t)
	model.focus = detailFocus
	model.selectedInstance = &unstructured.Unstructured{Object: map[string]interface{}{
		"kind": "ConfigMap",
		"data": map[string]interface{}{"state": "ready"},
	}}
	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, char := range "ready" {
		model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
	}
	if !model.searching {
		t.Fatal("detail search should remain editable until Enter")
	}
	if len(model.matches) != 1 {
		t.Fatalf("detail matches before Enter = %v, want one live match", model.matches)
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if model.searching {
		t.Fatal("Enter should close the detail search input")
	}
}

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
	if !strings.Contains(selected, "  - team *") {
		t.Fatalf("selected workspace row lost its tree marker or label: %s", selected)
	}
}

func TestPaneLabelAndUnfocusedSelectionStayVisible(t *testing.T) {
	model := testModel(t)
	model.resources = make([]kcp.Resource, 10)
	for i := range model.resources {
		model.resources[i] = kcp.Resource{Name: fmt.Sprintf("resource-%02d", i)}
	}
	model.resourceCursor = 7
	model.focus = workspaceFocus

	rendered := model.renderPane(resourceFocus, "API resources", model.resourceLines(), 32, 6, false)
	lines := strings.Split(ansi.Strip(rendered), "\n")
	if !strings.Contains(lines[0], "API resources") {
		t.Fatalf("pane label should be rendered in the top border: %q", lines[0])
	}
	if strings.Contains(lines[1], "API resources") {
		t.Fatalf("pane label should not consume a content row: %q", lines[1])
	}
	if !strings.Contains(rendered, "resource-07") || strings.Contains(rendered, "resource-00") {
		t.Fatalf("unfocused pane should keep its own selected item in view:\n%s", ansi.Strip(rendered))
	}

	model.resourceCursor = 6
	otherSelection := model.renderPane(resourceFocus, "API resources", model.resourceLines(), 32, 6, false)
	if rendered == otherSelection {
		t.Fatal("changing the unfocused resource cursor should change its visible selection style")
	}
}

func TestResourceScrollingDoesNotScrollWorkspacePane(t *testing.T) {
	model := testModel(t)
	model.root.loaded = true
	model.root.expanded = true
	model.activeWorkspace = model.root.path
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("ws-%02d", i)
		model.root.children = append(model.root.children, &workspaceNode{
			name: name,
			path: model.root.path + ":" + name,
		})
		model.resources = append(model.resources, kcp.Resource{Name: fmt.Sprintf("resource-%02d", i)})
	}
	model.focus = resourceFocus
	model.resourceCursor = 10

	workspacePane := ansi.Strip(model.renderPane(workspaceFocus, "Workspaces", model.workspaceLines(), 32, 6, false))
	if !strings.Contains(workspacePane, "root *") || !strings.Contains(workspacePane, "ws-00") {
		t.Fatalf("resource cursor scrolled the workspace pane away from its top:\n%s", workspacePane)
	}

	resourcePane := ansi.Strip(model.renderPane(resourceFocus, "API resources", model.resourceLines(), 32, 6, true))
	if !strings.Contains(resourcePane, "resource-10") || strings.Contains(resourcePane, "resource-00") {
		t.Fatalf("focused resource pane did not follow its own cursor:\n%s", resourcePane)
	}
}

func TestDetailFirstLayoutAndVisibilityToggle(t *testing.T) {
	model := testModel(t)
	model.width = 120
	model.height = 40
	model.activeWorkspace = strings.Repeat("long-workspace:", 12)

	view := ansi.Strip(model.View())
	lines := strings.Split(view, "\n")
	if got, want := lipgloss.Width(view), model.width; got != want {
		t.Fatalf("wide view width = %d, want %d", got, want)
	}
	if got, want := lipgloss.Height(view), model.height; got != want {
		t.Fatalf("view height = %d, want %d", got, want)
	}
	if got, want := lipgloss.Width(lines[1]), model.width; got != want {
		t.Fatalf("workspace pane width = %d, want %d", got, want)
	}
	if workspace, resources := strings.Index(view, "Workspaces"), strings.Index(view, "API resources"); workspace < 0 || resources < 0 || workspace > resources {
		t.Fatalf("workspace pane should render above navigation: %s", view)
	}
	detailColumn := func(rendered string) int {
		for _, line := range strings.Split(rendered, "\n") {
			if column := strings.Index(line, "Resource detail"); column >= 0 {
				return column
			}
		}
		return -1
	}
	normalDetailColumn := detailColumn(view)
	if normalDetailColumn < 0 {
		t.Fatal("normal layout is missing the detail pane")
	}

	model.focus = instanceFocus
	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if !model.detailOnly || model.focus != detailFocus {
		t.Fatalf("detail-only toggle state = (%v, %v), want (true, detailFocus)", model.detailOnly, model.focus)
	}
	detailView := ansi.Strip(model.View())
	if !strings.Contains(detailView, "Workspaces") || !strings.Contains(detailView, "Resource detail") {
		t.Fatalf("detail-only mode should retain workspace and detail panes: %s", detailView)
	}
	if strings.Contains(detailView, "API resources") || strings.Contains(detailView, "Instances") {
		t.Fatalf("detail-only mode should hide lower navigation panes: %s", detailView)
	}
	if detailColumn(detailView) >= normalDetailColumn {
		t.Fatalf("detail-only mode did not expand the detail pane: normal column %d, detail-only column %d", normalDetailColumn, detailColumn(detailView))
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if model.focus != workspaceFocus {
		t.Fatalf("Tab in detail-only mode focused %v, want workspaceFocus", model.focus)
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if model.detailOnly {
		t.Fatal("second toggle should restore the navigation panes")
	}
}

func TestNarrowLayoutRenders(t *testing.T) {
	model := testModel(t)
	model.width = 60
	model.height = 24
	view := ansi.Strip(model.View())
	if got, want := lipgloss.Height(view), model.height; got != want {
		t.Fatalf("narrow view height = %d, want %d", got, want)
	}
	if got, want := lipgloss.Width(view), model.width; got != want {
		t.Fatalf("narrow view width = %d, want %d", got, want)
	}
}

func TestVerySmallTerminalMessageFits(t *testing.T) {
	model := testModel(t)
	model.width = 24
	model.height = 10
	view := ansi.Strip(model.View())
	if got, want := lipgloss.Width(view), model.width; got > want {
		t.Fatalf("small-terminal message width = %d, exceeds %d", got, want)
	}
}

func TestCtrlDUPMovesListSelectionsByHalfPane(t *testing.T) {
	model := testModel(t)
	model.width = 120
	model.height = 40
	model.root.loaded = true
	model.root.expanded = true
	for i := 0; i < 30; i++ {
		name := fmt.Sprintf("ws-%02d", i)
		model.root.children = append(model.root.children, &workspaceNode{
			name: name,
			path: model.root.path + ":" + name,
		})
		model.resources = append(model.resources, kcp.Resource{Name: fmt.Sprintf("resource-%02d", i)})
		model.instances = append(model.instances, unstructured.Unstructured{
			Object: map[string]interface{}{"metadata": map[string]interface{}{"name": fmt.Sprintf("instance-%02d", i)}},
		})
	}

	model.focus = workspaceFocus
	model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	if model.workspaceCursor != "root:ws-01" {
		t.Fatalf("workspace Ctrl+D cursor = %q, want second child after half-page step", model.workspaceCursor)
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlU})
	if model.workspaceCursor != model.root.path {
		t.Fatalf("workspace Ctrl+U cursor = %q, want root", model.workspaceCursor)
	}

	model.focus = resourceFocus
	model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	if model.resourceCursor != 6 {
		t.Fatalf("resource cursor after Ctrl+D = %d, want 6", model.resourceCursor)
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlU})
	if model.resourceCursor != 0 {
		t.Fatalf("resource cursor after Ctrl+U = %d, want 0", model.resourceCursor)
	}

	model.focus = instanceFocus
	model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	if model.instanceCursor != 7 {
		t.Fatalf("instance cursor after Ctrl+D = %d, want 7", model.instanceCursor)
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlU})
	if model.instanceCursor != 0 {
		t.Fatalf("instance cursor after Ctrl+U = %d, want 0", model.instanceCursor)
	}
}

func TestCtrlDUPScrollsDetailsAndClamps(t *testing.T) {
	model := testModel(t)
	model.width = 120
	model.height = 40
	data := make(map[string]interface{})
	for i := 0; i < 100; i++ {
		data[fmt.Sprintf("entry-%02d", i)] = "value"
	}
	model.selectedInstance = &unstructured.Unstructured{Object: map[string]interface{}{"data": data}}
	model.focus = detailFocus

	model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	if model.scroll != 14 {
		t.Fatalf("detail scroll after Ctrl+D = %d, want 14", model.scroll)
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlU})
	if model.scroll != 0 {
		t.Fatalf("detail scroll after Ctrl+U = %d, want 0", model.scroll)
	}

	maxScroll := len(model.detailLines()) - model.paneVisibleRows(detailFocus)
	for i := 0; i < 20; i++ {
		model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	}
	if model.scroll != maxScroll {
		t.Fatalf("detail scroll at end = %d, want clamped maximum %d", model.scroll, maxScroll)
	}
}

func TestCtrlDUsesResponsiveFocusedPaneHeight(t *testing.T) {
	model := testModel(t)
	model.width = 60
	model.height = 24
	model.focus = resourceFocus
	model.resources = make([]kcp.Resource, 20)
	model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	if model.resourceCursor != 1 {
		t.Fatalf("narrow resource cursor after Ctrl+D = %d, want 1", model.resourceCursor)
	}

	model.detailOnly = true
	model.focus = detailFocus
	if got, want := model.paneVisibleRows(detailFocus), 15; got != want {
		t.Fatalf("detail-only visible rows = %d, want %d", got, want)
	}
}

func TestMouseWheelTargetsHoveredPane(t *testing.T) {
	model := testModel(t)
	model.width = 120
	model.height = 40
	model.root.loaded = true
	model.root.expanded = true
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("ws-%02d", i)
		model.root.children = append(model.root.children, &workspaceNode{name: name, path: "root:" + name})
		model.resources = append(model.resources, kcp.Resource{Name: fmt.Sprintf("resource-%02d", i)})
	}
	data := make(map[string]interface{})
	for i := 0; i < 30; i++ {
		data[fmt.Sprintf("entry-%02d", i)] = "value"
	}
	model.selectedInstance = &unstructured.Unstructured{Object: map[string]interface{}{"data": data}}

	layout := model.layoutForView()
	lowerY := 1 + layout.workspaceHeight
	workspaceCursor := model.workspaceCursor
	sendMouse(model, tea.MouseMsg{
		X:      1,
		Y:      lowerY + 2,
		Button: tea.MouseButtonWheelDown,
	})
	if model.focus != resourceFocus || model.resourceCursor != 3 {
		t.Fatalf("resource wheel state = focus %v, cursor %d; want resource focus and cursor 3", model.focus, model.resourceCursor)
	}
	if model.workspaceCursor != workspaceCursor {
		t.Fatalf("resource wheel changed workspace cursor to %q", model.workspaceCursor)
	}

	sendMouse(model, tea.MouseMsg{
		X:      layout.navigationWidth + 1,
		Y:      lowerY + 2,
		Button: tea.MouseButtonWheelDown,
	})
	if model.focus != detailFocus || model.scroll != 3 {
		t.Fatalf("detail wheel state = focus %v, scroll %d; want detail focus and scroll 3", model.focus, model.scroll)
	}
	sendMouse(model, tea.MouseMsg{
		X:      layout.navigationWidth + 1,
		Y:      lowerY + 2,
		Button: tea.MouseButtonWheelUp,
	})
	if model.scroll != 0 {
		t.Fatalf("wheel up scroll = %d, want clamped zero", model.scroll)
	}

	model.detailOnly = true
	model.focus = workspaceFocus
	sendMouse(model, tea.MouseMsg{
		X:      1,
		Y:      lowerY + 2,
		Button: tea.MouseButtonWheelDown,
	})
	if model.focus != detailFocus || model.scroll != 3 {
		t.Fatalf("detail-only wheel state = focus %v, scroll %d; want detail focus and scroll 3", model.focus, model.scroll)
	}
}

func TestMouseClicksActivateListItems(t *testing.T) {
	model := testModel(t)
	model.width = 120
	model.height = 40
	model.resources = []kcp.Resource{
		{Name: "pods", Kind: "Pod"},
		{Name: "deployments", Kind: "Deployment", Group: "apps"},
	}
	layout := model.layoutForView()
	lowerY := 1 + layout.workspaceHeight
	command := sendMouse(model, tea.MouseMsg{
		X:      1,
		Y:      lowerY + 2,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if command == nil {
		t.Fatal("clicking a resource should start loading its instances")
	}
	if model.focus != resourceFocus || model.resourceCursor != 1 || model.selectedResource == nil || model.selectedResource.Name != "deployments" {
		t.Fatalf("resource click state = focus %v, cursor %d, resource %#v", model.focus, model.resourceCursor, model.selectedResource)
	}

	model.instances = []unstructured.Unstructured{
		{Object: map[string]interface{}{"metadata": map[string]interface{}{"name": "first"}}},
		{Object: map[string]interface{}{"metadata": map[string]interface{}{"name": "second"}}},
	}
	instanceY := lowerY + layout.resourceHeight + 2
	sendMouse(model, tea.MouseMsg{
		X:      1,
		Y:      instanceY,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if model.focus != instanceFocus || model.selectedInstance == nil || model.selectedInstance.GetName() != "second" {
		t.Fatalf("instance click state = focus %v, instance %#v", model.focus, model.selectedInstance)
	}
	if !strings.Contains(strings.Join(model.detailLines(), "\n"), "second") {
		t.Fatal("clicking an instance should update the detail view")
	}
}

func TestKeyboardEnterOnInstanceFocusesDetails(t *testing.T) {
	model := testModel(t)
	model.instances = []unstructured.Unstructured{
		{Object: map[string]interface{}{"metadata": map[string]interface{}{"name": "selected"}}},
	}
	model.focus = instanceFocus

	model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})

	if model.focus != detailFocus || model.selectedInstance == nil || model.selectedInstance.GetName() != "selected" {
		t.Fatalf("keyboard instance activation state = focus %v, instance %#v; want details focus and selected instance", model.focus, model.selectedInstance)
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
	if !strings.HasPrefix(model.workspaceLines()[1], "  -") {
		t.Fatalf("expanded workspace marker not rendered: %#v", model.workspaceLines())
	}
	sendMouse(model, markerClick)
	if child.expanded {
		t.Fatal("clicking an expanded workspace marker should collapse it")
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

func TestMouseClickUsesNarrowLayoutCoordinates(t *testing.T) {
	model := testModel(t)
	model.width = 60
	model.height = 24
	model.resources = []kcp.Resource{{Name: "pods", Kind: "Pod"}}
	layout := model.layoutForView()
	lowerY := 1 + layout.workspaceHeight
	sendMouse(model, tea.MouseMsg{
		X:      1,
		Y:      lowerY + 1,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if model.focus != resourceFocus || model.selectedResource == nil || model.selectedResource.Name != "pods" {
		t.Fatalf("narrow-layout resource click state = focus %v, resource %#v", model.focus, model.selectedResource)
	}
}
