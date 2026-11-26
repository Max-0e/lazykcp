package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Max-0e/lazykcp/internal/kcp"
	tea "github.com/charmbracelet/bubbletea"
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

func TestInitDiscoversResourcesForRootWorkspace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/clusters/root/apis":
			_, _ = fmt.Fprint(w, `{"apiVersion":"v1","kind":"APIGroupList","groups":[]}`)
		case "/clusters/root/api":
			_, _ = fmt.Fprint(w, `{"apiVersion":"v1","kind":"APIVersions","versions":["v1"]}`)
		case "/clusters/root/api/v1":
			_, _ = fmt.Fprint(w, `{"apiVersion":"v1","kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"pods","singularName":"pod","namespaced":true,"kind":"Pod","verbs":["get","list"]}]}`)
		case "/clusters/root/apis/tenancy.kcp.io/v1alpha1/workspaces":
			_, _ = fmt.Fprint(w, `{"apiVersion":"tenancy.kcp.io/v1alpha1","kind":"WorkspaceList","metadata":{"resourceVersion":"1"},"items":[]}`)
		default:
			t.Errorf("unexpected API request path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	path := t.TempDir() + "/config"
	config := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: kcp
  cluster:
    server: %s/clusters/root
users:
- name: admin
  user: {}
contexts:
- name: admin@kcp
  context:
    cluster: kcp
    user: admin
current-context: admin@kcp
`, server.URL)
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	client, err := kcp.NewClient(path, false)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	model := NewModel(client)

	message := model.Init()()
	batch, ok := message.(tea.BatchMsg)
	if !ok {
		t.Fatalf("Init() command message = %T, want a batch with workspace and resource discovery", message)
	}
	if len(batch) != 2 {
		t.Fatalf("Init() batch has %d commands, want workspace loading and root resource discovery", len(batch))
	}
	for _, command := range batch {
		result := command()
		if _, followup := model.Update(result); followup != nil {
			t.Fatalf("startup result unexpectedly returned a follow-up command: %v", followup)
		}
	}

	if model.activeWorkspace != "root" {
		t.Fatalf("initial active workspace = %q, want root", model.activeWorkspace)
	}
	if len(model.resources) != 1 || model.resources[0].Name != "pods" {
		t.Fatalf("root resources after initialization = %#v, want pods", model.resources)
	}
	if !model.root.loaded {
		t.Fatal("root workspace was not loaded during initialization")
	}
}

func sendMouse(model *Model, message tea.MouseMsg) tea.Cmd {
	_, command := model.Update(message)
	return command
}

func dragPaneDivider(t *testing.T, model *Model, startX, startY, endX, endY int) {
	t.Helper()
	sendMouse(model, tea.MouseMsg{
		X:      startX,
		Y:      startY,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	if model.dragDivider == noDivider {
		t.Fatalf("mouse press at (%d, %d) did not start a divider drag", startX, startY)
	}
	sendMouse(model, tea.MouseMsg{
		X:      endX,
		Y:      endY,
		Action: tea.MouseActionMotion,
		Button: tea.MouseButtonNone,
	})
	sendMouse(model, tea.MouseMsg{
		X:      endX,
		Y:      endY,
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	})
}
