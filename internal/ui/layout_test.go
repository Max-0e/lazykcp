package ui

import (
	"strings"
	"testing"

	"github.com/Max-0e/lazykcp/internal/kcp"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

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

func TestWidePaneDividersResizeAndPreserveLayout(t *testing.T) {
	model := testModel(t)
	model.width = 120
	model.height = 40

	layout := model.layoutForView()
	workspaceBoundary := 1 + layout.workspaceHeight
	dragPaneDivider(t, model, 50, workspaceBoundary-1, 50, workspaceBoundary+2)
	if got, want := model.layoutForView().workspaceHeight, layout.workspaceHeight+3; got != want {
		t.Fatalf("workspace height after drag = %d, want %d", got, want)
	}

	layout = model.layoutForView()
	lowerY := 1 + layout.workspaceHeight
	dragPaneDivider(t, model, layout.navigationWidth-1, lowerY+2, layout.navigationWidth+4, lowerY+2)
	if got, want := model.layoutForView().navigationWidth, layout.navigationWidth+5; got != want {
		t.Fatalf("navigation width after drag = %d, want %d", got, want)
	}

	layout = model.layoutForView()
	lowerY = 1 + layout.workspaceHeight
	resourceBoundary := lowerY + layout.resourceHeight
	dragPaneDivider(t, model, 1, resourceBoundary-1, 1, resourceBoundary+2)
	resized := model.layoutForView()
	if got, want := resized.resourceHeight, layout.resourceHeight+3; got != want {
		t.Fatalf("resource height after drag = %d, want %d", got, want)
	}
	if resized.resourceHeight+resized.instanceHeight != resized.detailHeight {
		t.Fatalf("wide navigation pane heights do not fill lower section: %#v", resized)
	}
	if resized.navigationWidth+resized.detailWidth != model.width {
		t.Fatalf("wide pane widths do not fill terminal: %#v", resized)
	}
	if got, want := lipgloss.Width(ansi.Strip(model.View())), model.width; got != want {
		t.Fatalf("resized wide view width = %d, want %d", got, want)
	}
	if got, want := lipgloss.Height(ansi.Strip(model.View())), model.height; got != want {
		t.Fatalf("resized wide view height = %d, want %d", got, want)
	}
}

func TestNarrowPaneDividersResizeAndPreserveLayout(t *testing.T) {
	model := testModel(t)
	model.width = 60
	model.height = 24
	layout := model.layoutForView()
	lowerY := 1 + layout.workspaceHeight

	dragPaneDivider(t, model, layout.resourceWidth-1, lowerY+1, layout.resourceWidth+3, lowerY+1)
	resized := model.layoutForView()
	if got, want := resized.resourceWidth, layout.resourceWidth+4; got != want {
		t.Fatalf("resource width after drag = %d, want %d", got, want)
	}

	lowerY = 1 + resized.workspaceHeight
	navigationBoundary := lowerY + resized.navigationHeight
	dragPaneDivider(t, model, 5, navigationBoundary-1, 5, navigationBoundary+1)
	resized = model.layoutForView()
	if got, want := resized.navigationHeight, layout.navigationHeight+2; got != want {
		t.Fatalf("navigation height after drag = %d, want %d", got, want)
	}
	if resized.navigationHeight+resized.detailHeight != model.height-2-resized.workspaceHeight {
		t.Fatalf("narrow pane heights do not fill lower section: %#v", resized)
	}
	if resized.resourceWidth+resized.instanceWidth != model.width {
		t.Fatalf("narrow pane widths do not fill terminal: %#v", resized)
	}
	if got, want := lipgloss.Width(ansi.Strip(model.View())), model.width; got != want {
		t.Fatalf("resized narrow view width = %d, want %d", got, want)
	}
	if got, want := lipgloss.Height(ansi.Strip(model.View())), model.height; got != want {
		t.Fatalf("resized narrow view height = %d, want %d", got, want)
	}
}

func TestPaneResizeClampsAndAdaptsToTerminalSize(t *testing.T) {
	model := testModel(t)
	model.width = 120
	model.height = 40

	layout := model.layoutForView()
	lowerY := 1 + layout.workspaceHeight
	dragPaneDivider(t, model, layout.navigationWidth-1, lowerY+2, 0, lowerY+2)
	if got := model.layoutForView().navigationWidth; got != minNavigationWidth {
		t.Fatalf("navigation width at minimum = %d, want %d", got, minNavigationWidth)
	}

	layout = model.layoutForView()
	workspaceBoundary := 1 + layout.workspaceHeight
	dragPaneDivider(t, model, 50, workspaceBoundary-1, 50, -20)
	if got := model.layoutForView().workspaceHeight; got != 5 {
		t.Fatalf("workspace height at minimum = %d, want 5", got)
	}

	model = testModel(t)
	model.width = 120
	model.height = 40
	layout = model.layoutForView()
	lowerY = 1 + layout.workspaceHeight
	dragPaneDivider(t, model, layout.navigationWidth-1, lowerY+2, 79, lowerY+2)
	if got := model.layoutForView().navigationWidth; got != 80 {
		t.Fatalf("navigation width after expansion = %d, want 80", got)
	}
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	if got := model.layoutForView().navigationWidth; got != 60 {
		t.Fatalf("navigation width after terminal shrink = %d, want 60", got)
	}
	model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if got := model.layoutForView().navigationWidth; got != 80 {
		t.Fatalf("navigation width after terminal restore = %d, want 80", got)
	}

	model.detailOnly = true
	layout = model.layoutForView()
	lowerY = 1 + layout.workspaceHeight
	if _, ok := model.dividerAt(0, lowerY+2); ok {
		t.Fatal("hidden navigation dividers should not be resizable in detail-only mode")
	}
	workspaceBoundary = 1 + layout.workspaceHeight
	dragPaneDivider(t, model, 50, workspaceBoundary-1, 50, workspaceBoundary)
	if got := model.layoutForView().workspaceHeight; got != layout.workspaceHeight+1 {
		t.Fatalf("workspace should remain resizable in detail-only mode: got %d, want %d", got, layout.workspaceHeight+1)
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
