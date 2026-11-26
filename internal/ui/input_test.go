package ui

import (
	"fmt"
	"testing"

	"github.com/Max-0e/lazykcp/internal/kcp"
	tea "github.com/charmbracelet/bubbletea"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

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
