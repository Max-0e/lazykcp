package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Max-0e/lazykcp/internal/kcp"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

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

func TestEmptyNavigationPanesShowPlaceholders(t *testing.T) {
	model := testModel(t)
	tests := []struct {
		panel       focus
		title       string
		placeholder string
	}{
		{panel: resourceFocus, title: "API resources", placeholder: "No API resources"},
		{panel: instanceFocus, title: "Instances", placeholder: "Select a resource"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			rendered := ansi.Strip(model.renderPane(tt.panel, tt.title, model.linesForPanel(tt.panel), 32, 6, true))
			if !strings.Contains(rendered, tt.placeholder) {
				t.Fatalf("empty %s pane is missing placeholder %q:\n%s", tt.title, tt.placeholder, rendered)
			}
			if model.selectedLineIndex(tt.panel) != -1 {
				t.Fatalf("empty %s pane should not have a selectable placeholder row", tt.title)
			}
		})
	}

	model.selectedResource = &kcp.Resource{Name: "pods"}
	model.loading = ""
	rendered := ansi.Strip(model.renderPane(instanceFocus, "Instances", model.instanceLines(), 32, 6, false))
	if !strings.Contains(rendered, "No instances") {
		t.Fatalf("empty instances for a selected API resource should be explicit:\n%s", rendered)
	}
	model.loading = "Loading instances"
	rendered = ansi.Strip(model.renderPane(instanceFocus, "Instances", model.instanceLines(), 32, 6, false))
	if !strings.Contains(rendered, "Loading instances") {
		t.Fatalf("selected resource should show loading state while instances are fetched:\n%s", rendered)
	}
}

func TestClickingEmptyPanePlaceholderDoesNotSelectItem(t *testing.T) {
	model := testModel(t)
	model.width = 120
	model.height = 40
	layout := model.layoutForView()
	lowerY := 1 + layout.workspaceHeight

	resourceCommand := sendMouse(model, tea.MouseMsg{
		X:      1,
		Y:      lowerY + 1,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if resourceCommand != nil || model.selectedResource != nil {
		t.Fatalf("resource placeholder click selected an item or returned a command: resource %#v, command %v", model.selectedResource, resourceCommand)
	}

	instanceY := lowerY + layout.resourceHeight + 1
	instanceCommand := sendMouse(model, tea.MouseMsg{
		X:      1,
		Y:      instanceY,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if instanceCommand != nil || model.selectedInstance != nil {
		t.Fatalf("instance placeholder click selected an item or returned a command: instance %#v, command %v", model.selectedInstance, instanceCommand)
	}
}

func TestMouseClickDetailTabsSwitchRepresentation(t *testing.T) {
	model := testModel(t)
	model.width = 120
	model.height = 40
	model.selectedInstance = &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]interface{}{"name": "settings"},
		"data":       map[string]interface{}{"theme": "dark"},
	}}
	model.detailView = jsonDetailView
	model.focus = instanceFocus

	layout := model.layoutForView()
	lowerY := 1 + layout.workspaceHeight
	bounds, ok := model.paneBoundsAt(layout.navigationWidth+1, lowerY+1)
	if !ok || bounds.panel != detailFocus {
		t.Fatal("could not locate the detail pane")
	}
	innerWidth := bounds.width - 2
	_, overviewWidth, jsonWidth, _, _ := detailTabsGeometry(innerWidth)
	tabStart := bounds.x + 1 + innerWidth - detailTabsCompactWidth
	if innerWidth >= detailTabsFullMinInnerWidth {
		tabStart = bounds.x + 1 + innerWidth - detailTabsFullWidth
	}
	clickTab := func(offset int) {
		t.Helper()
		sendMouse(model, tea.MouseMsg{
			X:      tabStart + offset,
			Y:      bounds.y,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonLeft,
		})
	}

	clickTab(1)
	if model.detailView != overviewDetailView || model.focus != detailFocus {
		t.Fatalf("Overview tab click state = tab %d, focus %v; want Overview and details focus", model.detailView, model.focus)
	}
	if got := strings.Join(model.detailLines(), "\n"); !strings.Contains(got, "Kind: ConfigMap") {
		t.Fatalf("Overview tab click did not show the summary: %s", got)
	}

	clickTab(overviewWidth + 1 + 1)
	if model.detailView != jsonDetailView || model.focus != detailFocus {
		t.Fatalf("JSON tab click state = tab %d, focus %v; want JSON and details focus", model.detailView, model.focus)
	}
	if !strings.Contains(strings.Join(model.detailLines(), "\n"), `"theme": "dark"`) {
		t.Fatal("JSON tab click did not update the representation")
	}

	clickTab(overviewWidth + jsonWidth + 2 + 1)
	if model.detailView != yamlDetailView || model.focus != detailFocus {
		t.Fatalf("YAML tab click state = tab %d, focus %v; want YAML and details focus", model.detailView, model.focus)
	}
	if !strings.Contains(strings.Join(model.detailLines(), "\n"), "theme: dark") {
		t.Fatal("YAML tab click did not update the representation")
	}
	yamlHeader := ansi.Strip(strings.Split(model.renderPane(detailFocus, model.detailTitle(), model.detailLines(), bounds.width, bounds.height, true), "\n")[0])
	model.setDetailTab(jsonDetailView)
	jsonHeader := ansi.Strip(strings.Split(model.renderPane(detailFocus, model.detailTitle(), model.detailLines(), bounds.width, bounds.height, true), "\n")[0])
	if jsonHeader == yamlHeader {
		t.Fatal("active JSON/YAML tabs should have distinct visual styling")
	}
	if !strings.Contains(jsonHeader, "[JSON]") || !strings.Contains(yamlHeader, "[YAML]") {
		t.Fatalf("detail tabs were not visibly rendered with the active format selected: JSON %q, YAML %q", jsonHeader, yamlHeader)
	}
}

func TestScrollThumbTracksViewport(t *testing.T) {
	tests := []struct {
		name        string
		start       int
		visibleRows int
		lineCount   int
		wantTop     int
		wantHeight  int
		wantScroll  bool
	}{
		{name: "top", start: 0, visibleRows: 4, lineCount: 20, wantTop: 0, wantHeight: 1, wantScroll: true},
		{name: "middle", start: 8, visibleRows: 4, lineCount: 20, wantTop: 1, wantHeight: 1, wantScroll: true},
		{name: "bottom", start: 16, visibleRows: 4, lineCount: 20, wantTop: 3, wantHeight: 1, wantScroll: true},
		{name: "fits", start: 0, visibleRows: 4, lineCount: 4, wantScroll: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTop, gotHeight, gotScroll := scrollThumb(tt.start, tt.visibleRows, tt.lineCount)
			if gotTop != tt.wantTop || gotHeight != tt.wantHeight || gotScroll != tt.wantScroll {
				t.Fatalf("scrollThumb() = (%d, %d, %v), want (%d, %d, %v)",
					gotTop, gotHeight, gotScroll, tt.wantTop, tt.wantHeight, tt.wantScroll)
			}
		})
	}
}

func TestScrollablePanesRenderRightBorderIndicators(t *testing.T) {
	model := testModel(t)
	model.width = 32
	model.height = 6
	assertBottomThumb := func(t *testing.T, pane string) {
		t.Helper()
		lines := strings.Split(ansi.Strip(pane), "\n")
		lastContentRow := []rune(lines[len(lines)-2])
		if got := string(lastContentRow[len(lastContentRow)-1]); got != "█" {
			t.Fatalf("bottom viewport row right border = %q, want thumb marker █:\n%s", got, strings.Join(lines, "\n"))
		}
	}

	model.root.loaded = true
	model.root.expanded = true
	for i := 0; i < 19; i++ {
		name := fmt.Sprintf("ws-%02d", i)
		model.root.children = append(model.root.children, &workspaceNode{name: name, path: "root:" + name})
	}
	model.workspaceCursor = "root:ws-18"
	assertBottomThumb(t, model.renderPane(workspaceFocus, "Workspaces", model.workspaceLines(), 32, 6, false))

	model.resources = make([]kcp.Resource, 20)
	for i := range model.resources {
		model.resources[i] = kcp.Resource{Name: fmt.Sprintf("resource-%02d", i)}
	}
	model.resourceCursor = len(model.resources) - 1
	assertBottomThumb(t, model.renderPane(resourceFocus, "API resources", model.resourceLines(), 32, 6, false))
	model.resourceView = groupedResourceView
	model.expandedResourceGroups = map[string]bool{"": true}
	model.resourceCursor = len(model.resourceLines()) - 1
	assertBottomThumb(t, model.renderPane(resourceFocus, "API resources", model.resourceLines(), 32, 6, false))
	model.resourceView = flatResourceView

	model.instances = make([]unstructured.Unstructured, 20)
	for i := range model.instances {
		model.instances[i] = unstructured.Unstructured{Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": fmt.Sprintf("instance-%02d", i)},
		}}
	}
	model.instanceCursor = len(model.instances) - 1
	assertBottomThumb(t, model.renderPane(instanceFocus, "Instances", model.instanceLines(), 32, 6, false))

	data := make(map[string]interface{})
	for i := 0; i < 20; i++ {
		data[fmt.Sprintf("entry-%02d", i)] = "value"
	}
	model.selectedInstance = &unstructured.Unstructured{Object: map[string]interface{}{"data": data}}
	model.detailView = jsonDetailView
	model.scroll = len(model.detailLines()) - model.paneVisibleRows(detailFocus)
	assertBottomThumb(t, model.renderPane(detailFocus, model.detailTitle(), model.detailLines(), 32, 6, false))

	model.resources = model.resources[:4]
	model.resourceCursor = 0
	rendered := model.renderPane(resourceFocus, "API resources", model.resourceLines(), 32, 6, false)
	if strings.Contains(rendered, "┆") || strings.Contains(rendered, "█") {
		t.Fatalf("pane with content fitting its viewport should not render a scroll indicator:\n%s", ansi.Strip(rendered))
	}
}
