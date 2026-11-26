package ui

import (
	"strings"
	"testing"

	"github.com/Max-0e/lazykcp/internal/kcp"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

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
	model.detailView = jsonDetailView
	if got := strings.Join(model.detailLines(), "\n"); !strings.Contains(got, `"theme": "dark"`) {
		t.Fatalf("JSON detail missing instance data: %s", got)
	}
	model.searchText = "dark"
	model.findMatches()
	if len(model.matches) != 1 {
		t.Fatalf("JSON search matches = %v, want one match", model.matches)
	}
	model.setDetailTab(yamlDetailView)
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
	model.detailView = jsonDetailView
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

func TestPaneTitlesShowActiveFiltersAndDetailSearch(t *testing.T) {
	model := testModel(t)
	model.width = 120
	model.height = 40
	model.filters[workspaceFocus] = "root"
	model.filters[resourceFocus] = "pods"
	model.filters[instanceFocus] = "demo"
	model.searchText = "ready"
	model.selectedInstance = &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "settings"},
	}}

	view := ansi.Strip(model.View())
	for _, title := range []string{
		"Workspaces (/root)",
		"API resources (/pods)",
		"Instances (/demo)",
		"settings (/ready)",
	} {
		if !strings.Contains(view, title) {
			t.Errorf("view is missing active query title %q:\n%s", title, view)
		}
	}
}

func TestLongActiveQueryRemainsVisibleInNarrowPaneTitle(t *testing.T) {
	model := testModel(t)
	model.filters[resourceFocus] = "this-is-a-long-active-filter"

	rendered := ansi.Strip(model.renderPane(resourceFocus, "API resources", nil, 20, 6, false))
	header := strings.Split(rendered, "\n")[0]
	if got, want := lipgloss.Width(header), 20; got != want {
		t.Fatalf("narrow pane title width = %d, want %d", got, want)
	}
	if !strings.Contains(header, "(/this-is-") {
		t.Fatalf("active filter should remain visible when the title is constrained: %q", header)
	}
}

func TestActiveQueryTitlesUpdateAndClear(t *testing.T) {
	model := testModel(t)
	model.width = 120
	model.height = 40
	model.resources = []kcp.Resource{{Name: "pods", Kind: "Pod"}}
	model.focus = resourceFocus
	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, char := range "pods" {
		model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
	}
	if view := ansi.Strip(model.View()); !strings.Contains(view, "API resources (/pods)") {
		t.Fatalf("sidebar title did not update while typing:\n%s", view)
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if view := ansi.Strip(model.View()); !strings.Contains(view, "API resources (/pods)") {
		t.Fatalf("active filter title disappeared after closing input:\n%s", view)
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if view := ansi.Strip(model.View()); strings.Contains(view, "API resources (/pods)") {
		t.Fatalf("cleared filter remained in the pane title:\n%s", view)
	}

	model.selectedInstance = &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "settings"},
		"data":     map[string]interface{}{"state": "ready"},
	}}
	model.focus = detailFocus
	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, char := range "ready" {
		model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
	}
	if view := ansi.Strip(model.View()); !strings.Contains(view, "settings (/ready)") {
		t.Fatalf("detail title did not update while typing:\n%s", view)
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if view := ansi.Strip(model.View()); !strings.Contains(view, "settings (/ready)") {
		t.Fatalf("active detail search title disappeared after closing input:\n%s", view)
	}
	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if view := ansi.Strip(model.View()); strings.Contains(view, "settings (/ready)") {
		t.Fatalf("cleared detail search remained in the pane title:\n%s", view)
	}
}
