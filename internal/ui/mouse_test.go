package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Max-0e/lazykcp/internal/kcp"
	tea "github.com/charmbracelet/bubbletea"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

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
	model.detailView = jsonDetailView

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
