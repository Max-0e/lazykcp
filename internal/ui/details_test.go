package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestOverviewIsDefaultAndSummarizesSelectedInstance(t *testing.T) {
	model := testModel(t)
	if model.detailView != overviewDetailView {
		t.Fatalf("default detail view = %d, want Overview", model.detailView)
	}
	model.selectedInstance = &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":              "web",
			"namespace":         "production",
			"creationTimestamp": "2025-01-02T03:04:05Z",
			"labels":            map[string]interface{}{"z": "last", "app": "web"},
			"annotations":       map[string]interface{}{"description": "frontend"},
		},
		"status": map[string]interface{}{
			"readyReplicas": float64(2),
			"conditions": []interface{}{
				map[string]interface{}{
					"type":    "Available",
					"status":  "True",
					"reason":  "MinimumReplicasAvailable",
					"message": "Deployment has minimum availability.",
				},
			},
		},
		"spec": map[string]interface{}{
			"replicas": float64(3),
			"selector": map[string]interface{}{"matchLabels": map[string]interface{}{"app": "web"}},
		},
	}}

	got := strings.Join(model.detailLines(), "\n")
	for _, want := range []string{
		"IDENTITY",
		"Kind: Deployment",
		"API version: apps/v1",
		"Name: web",
		"Namespace: production",
		"Created: 2025-01-02T03:04:05Z",
		"Labels:",
		"app: web",
		"z: last",
		"Annotations:",
		"STATUS",
		"readyReplicas: 2",
		"Available: True; reason: MinimumReplicasAvailable; message: Deployment has minimum availability.",
		"SPEC",
		"replicas: 3",
		"selector: object (1 fields)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Overview missing %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "app: web") > strings.Index(got, "z: last") {
		t.Fatal("Overview labels should be sorted by key")
	}
	if strings.Index(got, "replicas: 3") > strings.Index(got, "selector:") {
		t.Fatal("Overview spec fields should be sorted by key")
	}
}

func TestOverviewOmitsAbsentSectionsAndHandlesMissingConditionType(t *testing.T) {
	model := testModel(t)
	model.selectedInstance = &unstructured.Unstructured{Object: map[string]interface{}{
		"kind": "Widget",
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"status": "Unknown"},
			},
		},
	}}

	got := strings.Join(model.detailLines(), "\n")
	if strings.Contains(got, "METADATA") || strings.Contains(got, "SPEC") {
		t.Fatalf("Overview should omit absent metadata/spec sections:\n%s", got)
	}
	if !strings.Contains(got, "Condition: Unknown") || strings.Contains(got, "<nil>") {
		t.Fatalf("missing condition type should use a readable fallback:\n%s", got)
	}
}

func TestOverviewSearchAndViewSelectionPersistence(t *testing.T) {
	model := testModel(t)
	model.focus = detailFocus
	model.selectedInstance = &unstructured.Unstructured{Object: map[string]interface{}{
		"kind":     "ConfigMap",
		"metadata": map[string]interface{}{"name": "settings"},
	}}
	model.handleKey(keyRune('o'))
	if model.detailView != overviewDetailView {
		t.Fatal("o in Details should select Overview")
	}

	model.searchText = "ConfigMap"
	model.findMatches()
	if len(model.matches) != 1 {
		t.Fatalf("Overview search matches = %v, want one", model.matches)
	}
	model.setDetailTab(yamlDetailView)
	if model.detailView != yamlDetailView || len(model.matches) != 1 {
		t.Fatalf("switching to YAML should persist selection and refresh search, got view %d matches %v", model.detailView, model.matches)
	}
	model.selectedInstance = &unstructured.Unstructured{Object: map[string]interface{}{
		"kind":     "Secret",
		"metadata": map[string]interface{}{"name": "credentials"},
	}}
	if model.detailView != yamlDetailView {
		t.Fatal("switching instances should preserve the selected detail view")
	}
}

func TestDetailTabsFitAndAreClickableAtCompactWidths(t *testing.T) {
	model := testModel(t)
	model.selectedInstance = &unstructured.Unstructured{Object: map[string]interface{}{
		"kind":     "ConfigMap",
		"metadata": map[string]interface{}{"name": "settings"},
	}}
	const width = 24
	rendered := model.renderPane(detailFocus, model.detailTitle(), model.detailLines(), width, 6, false)
	for _, line := range strings.Split(rendered, "\n") {
		if got := lipgloss.Width(ansi.Strip(line)); got > width {
			t.Fatalf("compact detail tab row width = %d, exceeds pane width %d: %q", got, width, line)
		}
	}

	bounds := paneBounds{panel: detailFocus, x: 0, y: 0, width: width, height: 6}
	innerWidth := width - 2
	totalWidth, _, _, _, compact := detailTabsGeometry(innerWidth)
	if !compact {
		t.Fatal("expected compact tab geometry")
	}
	start := bounds.x + 1 + innerWidth - totalWidth
	if tab, ok := detailTabAt(bounds, start+1); !ok || tab != overviewDetailView {
		t.Fatalf("compact first tab hit = (%d, %v), want Overview", tab, ok)
	}
	if tab, ok := detailTabAt(bounds, start+5); !ok || tab != jsonDetailView {
		t.Fatalf("compact second tab hit = (%d, %v), want JSON", tab, ok)
	}
	if tab, ok := detailTabAt(bounds, start+9); !ok || tab != yamlDetailView {
		t.Fatalf("compact third tab hit = (%d, %v), want YAML", tab, ok)
	}
}

func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}
