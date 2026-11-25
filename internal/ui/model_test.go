package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/Max-0e/lazykcp/internal/kcp"
	tea "github.com/charmbracelet/bubbletea"
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
	model.resources = []kcp.Resource{{Name: "pods", Kind: "Pod"}}
	model.focus = resourceFocus
	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p', 'o'}})
	model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if model.filters[resourceFocus] != "po" {
		t.Fatalf("resource filter = %q, want %q", model.filters[resourceFocus], "po")
	}
}
