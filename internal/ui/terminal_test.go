package ui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Max-0e/lazykcp/internal/kcp"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestTerminalKeyAndMouseEventMapping(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyMsg
		code rune
		text string
		mod  vt.KeyMod
	}{
		{name: "printable", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}}, code: 'A', text: "A"},
		{name: "control character", key: tea.KeyMsg{Type: tea.KeyCtrlC}, code: 'c', mod: vt.ModCtrl},
		{name: "up arrow", key: tea.KeyMsg{Type: tea.KeyUp}, code: vt.KeyUp},
		{name: "shift tab", key: tea.KeyMsg{Type: tea.KeyShiftTab}, code: vt.KeyTab, mod: vt.ModShift},
		{name: "function key", key: tea.KeyMsg{Type: tea.KeyF5}, code: vt.KeyF5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := terminalKeyEvent(tt.key)
			if !ok {
				t.Fatal("terminalKeyEvent() did not map the key")
			}
			if got.Code != tt.code || got.Text != tt.text || got.Mod != tt.mod {
				t.Fatalf("terminalKeyEvent() = %#v, want code %d text %q mod %d", got, tt.code, tt.text, tt.mod)
			}
		})
	}

	mouse, ok := terminalMouseEvent(tea.MouseMsg{
		X: 4, Y: 7, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	if !ok {
		t.Fatal("terminalMouseEvent() did not map a click")
	}
	if got := mouse.Mouse(); got.X != 4 || got.Y != 7 {
		t.Fatalf("terminal mouse coordinates = (%d, %d), want (4, 7)", got.X, got.Y)
	}
}

func TestTerminalStartsShellScopedToHighlightedWorkspace(t *testing.T) {
	model := testModel(t)
	model.width = 200
	model.height = 24
	model.focus = workspaceFocus
	model.root.children = []*workspaceNode{{name: "team", path: "root:team"}}
	model.workspaceCursor = "root:team"
	t.Setenv("SHELL", "/bin/sh")
	_, start := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if model.terminal == nil {
		t.Fatalf("t shortcut did not start terminal: %s", model.status)
	}
	session := model.terminal
	if session.workspace != "root:team" {
		t.Fatalf("terminal workspace = %q, want highlighted workspace root:team", session.workspace)
	}
	if _, err := os.Stat(session.kubeconfigPath); err != nil {
		t.Fatalf("temporary kubeconfig is unavailable while shell runs: %v", err)
	}

	startMessage := start()
	batch, ok := startMessage.(tea.BatchMsg)
	if !ok || len(batch) != 3 {
		t.Fatalf("terminal start command = %T, want input, output, and process commands", startMessage)
	}
	messages := make(chan tea.Msg, 8)
	for _, command := range batch {
		go func(command tea.Cmd) { messages <- command() }(command)
	}
	session.emulator.SendText(`printf 'KUBECONFIG=%s\n' "$KUBECONFIG"; printf 'PWD=%s\n' "$PWD"; exit` + "\n")

	deadline := time.After(8 * time.Second)
	for model.terminal != nil {
		select {
		case message := <-messages:
			_, next := model.Update(message)
			if next != nil {
				go func(command tea.Cmd) { messages <- command() }(next)
			}
		case <-deadline:
			_ = model.Close()
			t.Fatal("terminal shell did not exit after the queued commands")
		}
	}
	if model.focus != workspaceFocus {
		t.Fatalf("focus after shell exit = %v, want restored workspace focus", model.focus)
	}
	restoredView := ansi.Strip(model.View())
	if !strings.Contains(restoredView, "Workspaces") || !strings.Contains(restoredView, "Resource detail") ||
		strings.Contains(restoredView, "Terminal · root") {
		t.Fatalf("browser details were not restored after shell exit:\n%s", restoredView)
	}
	output := session.emulator.String()
	if !strings.Contains(output, "KUBECONFIG="+session.kubeconfigPath) {
		t.Fatalf("shell did not receive workspace KUBECONFIG:\n%s", output)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "PWD="+workingDirectory) {
		t.Fatalf("shell did not inherit the application working directory %q:\n%s", workingDirectory, output)
	}
	if _, err := os.Stat(session.kubeconfigPath); !os.IsNotExist(err) {
		t.Fatalf("temporary kubeconfig was not removed after shell exit: %v", err)
	}
}

func TestTerminalUsesDetailsPaneAndKeepsBrowserVisible(t *testing.T) {
	model := testModel(t)
	model.width = 120
	model.height = 40
	model.focus = workspaceFocus
	model.resources = []kcp.Resource{{Name: "pods", Version: "v1", Kind: "Pod"}}
	model.instances = []unstructured.Unstructured{{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "selected-pod"},
	}}}
	model.selectedResource = &model.resources[0]
	model.selectedInstance = &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "not shown in terminal mode"},
	}}
	t.Setenv("SHELL", "/bin/sh")
	start := model.startTerminal()
	if model.terminal == nil {
		t.Fatalf("terminal did not start: %s", model.status)
	}
	configPath := model.terminal.kubeconfigPath
	defer func() {
		if err := model.Close(); err != nil {
			t.Errorf("close terminal: %v", err)
		}
	}()
	rendered := ansi.Strip(model.View())
	for _, visible := range []string{"Workspaces", "API resource", "Instances", "Terminal · root"} {
		if !strings.Contains(rendered, visible) {
			t.Fatalf("browser/terminal layout is missing %q:\n%s", visible, rendered)
		}
	}
	for _, visible := range []string{"pods", "selected-pod"} {
		if !strings.Contains(rendered, visible) {
			t.Fatalf("terminal pane hid browser item %q:\n%s", visible, rendered)
		}
	}
	if got := lipgloss.Width(rendered); got != model.width {
		t.Fatalf("wide terminal browser view width = %d, want %d", got, model.width)
	}
	if got := lipgloss.Height(rendered); got != model.height {
		t.Fatalf("wide terminal browser view height = %d, want %d", got, model.height)
	}
	if strings.Contains(rendered, "not shown in terminal mode") {
		t.Fatalf("selected details should be replaced only within the terminal pane:\n%s", rendered)
	}
	if model.focus != detailFocus {
		t.Fatalf("focus while shell is active = %v, want detailFocus", model.focus)
	}
	if start == nil {
		t.Fatal("terminal start did not return asynchronous I/O commands")
	}
	model.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	narrow := ansi.Strip(model.View())
	for _, visible := range []string{"Workspaces", "API resource", "Instances", "Terminal · root"} {
		if !strings.Contains(narrow, visible) {
			t.Fatalf("narrow terminal browser view is missing %q:\n%s", visible, narrow)
		}
	}
	if got := lipgloss.Width(narrow); got != 60 {
		t.Fatalf("narrow terminal browser view width = %d, want 60", got)
	}
	if got := lipgloss.Height(narrow); got != 24 {
		t.Fatalf("narrow terminal browser view height = %d, want 24", got)
	}
	if err := model.Close(); err != nil {
		t.Fatalf("close terminal during application shutdown: %v", err)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("temporary kubeconfig was not removed during application shutdown: %v", err)
	}
	if model.focus != workspaceFocus {
		t.Fatalf("focus after application shutdown = %v, want restored workspace focus", model.focus)
	}
}

func TestTerminalStartFailureIsVisibleInBrowser(t *testing.T) {
	model := testModel(t)
	model.focus = workspaceFocus
	model.workspaceCursor = "missing"
	model.startTerminal()
	if model.terminal != nil || !strings.Contains(model.status, "no workspace is selected") {
		t.Fatalf("missing workspace failure should be visible, terminal=%v status=%q", model.terminal, model.status)
	}
}

func TestTerminalPTYSizeAndEnvironment(t *testing.T) {
	size := terminalPTYSize(120, 40)
	if size.Cols != 118 || size.Rows != 38 {
		t.Fatalf("terminal PTY size = %dx%d, want pane interior 118x38", size.Cols, size.Rows)
	}
	if got := terminalPTYSize(0, 0); got.Cols != 1 || got.Rows != 1 {
		t.Fatalf("minimum PTY size = %dx%d, want 1x1", got.Cols, got.Rows)
	}

	got := terminalEnvironment([]string{"PATH=/bin", "TERM=dumb", "KUBECONFIG=/old"}, "/tmp/new-config")
	if strings.Count(strings.Join(got, "\n"), "TERM=") != 1 ||
		strings.Count(strings.Join(got, "\n"), "KUBECONFIG=") != 1 {
		t.Fatalf("terminal environment did not replace terminal-specific variables: %#v", got)
	}
	if !strings.Contains(strings.Join(got, "\n"), "TERM=xterm-256color") ||
		!strings.Contains(strings.Join(got, "\n"), "KUBECONFIG=/tmp/new-config") {
		t.Fatalf("terminal environment missing scoped settings: %#v", got)
	}
}

func TestTerminalPTYSizeTracksResponsiveDetailsPane(t *testing.T) {
	model := testModel(t)
	tests := []struct {
		name        string
		width       int
		height      int
		detailOnly  bool
		wantColumns int
	}{
		{name: "wide", width: 120, height: 40, wantColumns: 85},
		{name: "narrow", width: 60, height: 24, wantColumns: 58},
		{name: "detail only", width: 120, height: 40, detailOnly: true, wantColumns: 118},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model.width, model.height, model.detailOnly = tt.width, tt.height, tt.detailOnly
			bounds := model.detailPaneBounds()
			size := model.terminalPTYSize()
			if got, want := int(size.Cols), bounds.width-2; got != want || got != tt.wantColumns {
				t.Fatalf("terminal columns = %d, want %d (detail pane width %d)", got, tt.wantColumns, bounds.width)
			}
			if got, want := int(size.Rows), bounds.height-2; got != want {
				t.Fatalf("terminal rows = %d, want %d (detail pane height %d)", got, want, bounds.height)
			}
		})
	}
}

func TestTerminalMouseCoordinatesArePaneLocal(t *testing.T) {
	bounds := paneBounds{panel: detailFocus, x: 30, y: 12, width: 50, height: 20}
	message, ok := terminalPaneMouseMessage(bounds, tea.MouseMsg{X: 35, Y: 16})
	if !ok || message.X != 4 || message.Y != 3 {
		t.Fatalf("pane-local mouse message = (%d, %d), %v; want (4, 3), true", message.X, message.Y, ok)
	}
	for _, point := range [][2]int{{30, 16}, {80, 16}, {35, 12}, {35, 32}, {10, 4}} {
		if _, ok := terminalPaneMouseMessage(bounds, tea.MouseMsg{X: point[0], Y: point[1]}); ok {
			t.Fatalf("mouse event outside terminal content at (%d, %d) was accepted", point[0], point[1])
		}
	}
}

func ExampleterminalEnvironment() {
	fmt.Println(strings.Join(terminalEnvironment([]string{"PATH=/bin"}, "/tmp/kubeconfig"), ","))
	// Output: PATH=/bin,KUBECONFIG=/tmp/kubeconfig,TERM=xterm-256color
}
