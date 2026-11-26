package ui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

type detailView int

const (
	overviewDetailView detailView = iota
	jsonDetailView
	yamlDetailView
)

func (m *Model) detailTitle() string {
	if m.selectedInstance == nil {
		return "Resource detail"
	}
	return m.selectedInstance.GetName()
}

func (m *Model) detailLines() []string {
	if m.selectedInstance == nil {
		return []string{"Select an instance to inspect"}
	}
	switch m.detailView {
	case overviewDetailView:
		return m.overviewLines()
	case jsonDetailView:
		data, err := json.MarshalIndent(m.selectedInstance.Object, "", "  ")
		if err != nil {
			return []string{"Unable to serialize object: " + err.Error()}
		}
		return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	case yamlDetailView:
		jsonData, err := json.Marshal(m.selectedInstance.Object)
		if err == nil {
			var data []byte
			data, err = yaml.JSONToYAML(jsonData)
			if err == nil {
				return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
			}
		}
		return []string{"Unable to serialize object: " + err.Error()}
	default:
		return []string{"Unable to render resource detail"}
	}
}

func (m *Model) overviewLines() []string {
	instance := m.selectedInstance
	if instance == nil {
		return []string{"Select an instance to inspect"}
	}
	lines := []string{
		"IDENTITY",
		"  Kind: " + overviewText(instance.GetKind(), "Unknown"),
		"  API version: " + overviewText(instance.GetAPIVersion(), "Unknown"),
		"  Name: " + overviewText(instance.GetName(), "Unnamed"),
	}
	if namespace := instance.GetNamespace(); namespace != "" {
		lines = append(lines, "  Namespace: "+namespace)
	}

	metadataLines := make([]string, 0)
	if created := instance.GetCreationTimestamp(); !created.IsZero() {
		metadataLines = append(metadataLines, "  Created: "+created.Time.UTC().Format(time.RFC3339))
	}
	metadataLines = appendOverviewMap(metadataLines, "Labels", instance.GetLabels())
	metadataLines = appendOverviewMap(metadataLines, "Annotations", instance.GetAnnotations())
	if len(metadataLines) > 0 {
		lines = append(lines, "", "METADATA")
		lines = append(lines, metadataLines...)
	}

	status, found, err := unstructured.NestedMap(instance.Object, "status")
	if err == nil && found {
		statusLines := overviewStatusLines(status)
		if len(statusLines) > 0 {
			lines = append(lines, "", "STATUS")
			lines = append(lines, statusLines...)
		}
	}

	spec, found, err := unstructured.NestedMap(instance.Object, "spec")
	if err == nil && found && len(spec) > 0 {
		lines = append(lines, "", "SPEC")
		for _, key := range sortedOverviewKeys(spec) {
			lines = append(lines, "  "+key+": "+overviewValue(spec[key]))
		}
	}
	return lines
}

func overviewStatusLines(status map[string]interface{}) []string {
	lines := make([]string, 0, len(status))
	for _, key := range sortedOverviewKeys(status) {
		if key == "conditions" {
			continue
		}
		lines = append(lines, "  "+key+": "+overviewValue(status[key]))
	}
	conditions, ok := status["conditions"].([]interface{})
	if !ok {
		return lines
	}
	for _, value := range conditions {
		condition, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		conditionType, _ := condition["type"].(string)
		conditionType = overviewText(conditionType, "Condition")
		conditionStatus := overviewValue(condition["status"])
		line := "  " + conditionType + ": " + conditionStatus
		for _, key := range []string{"reason", "message"} {
			if value, ok := condition[key]; ok && value != nil && fmt.Sprint(value) != "" {
				line += "; " + key + ": " + overviewValue(value)
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func appendOverviewMap(lines []string, title string, values map[string]string) []string {
	if len(values) == 0 {
		return lines
	}
	lines = append(lines, "  "+title+":")
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, "    "+key+": "+overviewValue(values[key]))
	}
	return lines
}

func sortedOverviewKeys(values map[string]interface{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func overviewText(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func overviewValue(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case map[string]interface{}:
		return fmt.Sprintf("object (%d fields)", len(typed))
	case []interface{}:
		return fmt.Sprintf("list (%d items)", len(typed))
	case string:
		return clip(typed, 120)
	default:
		return fmt.Sprint(value)
	}
}

func overviewSectionHeading(line string) bool {
	switch line {
	case "IDENTITY", "METADATA", "STATUS", "SPEC":
		return true
	default:
		return false
	}
}
