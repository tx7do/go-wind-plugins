package assets

import (
	"strings"
	"testing"
)

// requiredSections lists the sections every casbin model file must declare.
var requiredSections = []string{
	"[request_definition]",
	"[policy_definition]",
	"[policy_effect]",
	"[matchers]",
}

// models maps the embedded model variables to the expectations for each one.
type modelExpectation struct {
	name           string
	content        string
	hasRoleSection bool
}

func allModels() []modelExpectation {
	return []modelExpectation{
		{name: "DefaultRbacModel", content: DefaultRbacModel, hasRoleSection: true},
		{name: "DefaultRbacWithDomainModel", content: DefaultRbacWithDomainModel, hasRoleSection: true},
		{name: "DefaultAbacModel", content: DefaultAbacModel, hasRoleSection: false},
		{name: "DefaultAclModel", content: DefaultAclModel, hasRoleSection: false},
		{name: "DefaultRestfullModel", content: DefaultRestfullModel, hasRoleSection: false},
		{name: "DefaultRestfullWithRoleModel", content: DefaultRestfullWithRoleModel, hasRoleSection: true},
	}
}

func TestEmbeddedModelsAreNonEmpty(t *testing.T) {
	for _, m := range allModels() {
		m := m
		t.Run(m.name, func(t *testing.T) {
			if strings.TrimSpace(m.content) == "" {
				t.Fatalf("embedded %s is empty", m.name)
			}
		})
	}
}

func TestEmbeddedModelsContainRequiredSections(t *testing.T) {
	for _, m := range allModels() {
		m := m
		t.Run(m.name, func(t *testing.T) {
			for _, section := range requiredSections {
				if !strings.Contains(m.content, section) {
					t.Errorf("%s is missing required section %s", m.name, section)
				}
			}
		})
	}
}

func TestEmbeddedModelsRoleSection(t *testing.T) {
	for _, m := range allModels() {
		m := m
		t.Run(m.name, func(t *testing.T) {
			hasRole := strings.Contains(m.content, "[role_definition]")
			if m.hasRoleSection && !hasRole {
				t.Errorf("%s should contain a [role_definition] section", m.name)
			}
			if !m.hasRoleSection && hasRole {
				t.Errorf("%s should not contain a [role_definition] section", m.name)
			}
		})
	}
}

func TestEmbeddedModelsSectionsHaveContent(t *testing.T) {
	// Every section must declare at least one non-comment line of content,
	// otherwise the model is unusable.
	for _, m := range allModels() {
		m := m
		t.Run(m.name, func(t *testing.T) {
			var currentSection string
			content := map[string][]string{}
			for _, line := range strings.Split(m.content, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
					continue
				}
				if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
					currentSection = line
					continue
				}
				if currentSection != "" {
					content[currentSection] = append(content[currentSection], line)
				}
			}
			for _, section := range requiredSections {
				if len(content[section]) == 0 {
					t.Errorf("%s: section %s has no content lines", m.name, section)
				}
			}
			if m.hasRoleSection && len(content["[role_definition]"]) == 0 {
				t.Errorf("%s: section [role_definition] has no content lines", m.name)
			}
		})
	}
}
