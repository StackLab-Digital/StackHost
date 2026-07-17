package composevalidator

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"gopkg.in/yaml.v3"
)

type Result struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
	Summary  Summary  `json:"summary"`
}

type Summary struct {
	Services             []string              `json:"services"`
	Images               []string              `json:"images"`
	Ports                []string              `json:"ports"`
	Volumes              []string              `json:"volumes"`
	Networks             []string              `json:"networks"`
	EnvironmentVariables []EnvironmentVariable `json:"environment_variables"`
}
type EnvironmentVariable struct {
	Name         string   `json:"name"`
	DefaultValue string   `json:"default_value"`
	Required     bool     `json:"required"`
	Secret       bool     `json:"secret"`
	Services     []string `json:"services"`
}

var variableReference = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?:(:?[-?])(.*?))?\}`)
var environmentKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func Validate(yaml string) Result {
	return ValidateWithEnvironment(yaml, nil)
}

func ValidateWithEnvironment(yaml string, environment map[string]string) Result {
	result := Result{Summary: Summary{Services: []string{}, Images: []string{}, Ports: []string{}, Volumes: []string{}, Networks: []string{}, EnvironmentVariables: []EnvironmentVariable{}}}
	if strings.TrimSpace(yaml) == "" {
		result.Errors = append(result.Errors, "Cole um arquivo Docker Compose para continuar.")
		return result
	}
	rawDocument := yamlValue(yaml)
	extractAllVariables(&result.Summary, rawDocument)
	composeEnvironment := types.Mapping{"COMPOSE_PROJECT_NAME": "stackhost"}
	for key, value := range environment {
		composeEnvironment[key] = value
	}
	config := types.ConfigDetails{WorkingDir: ".", ConfigFiles: []types.ConfigFile{{Filename: "stackhost.yml", Content: []byte(yaml)}}, Environment: composeEnvironment}
	project, err := loader.LoadWithContext(context.Background(), config, func(options *loader.Options) {
		options.SetProjectName("stackhost", true)
	})
	if err != nil {
		result.Summary.EnvironmentVariables = uniqueVariables(result.Summary.EnvironmentVariables)
		result.Errors = append(result.Errors, fmt.Sprintf("Docker Compose inválido: %v", err))
		return result
	}
	if len(project.Services) == 0 {
		result.Errors = append(result.Errors, "O Compose precisa definir pelo menos um serviço.")
		return result
	}
	for name, service := range project.Services {
		result.Summary.Services = append(result.Summary.Services, name)
		if service.Image == "" && service.Build != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("O serviço %q usa build, ainda não suportado nesta etapa.", name))
		} else if service.Image == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("O serviço %q precisa de uma imagem.", name))
		} else {
			result.Summary.Images = append(result.Summary.Images, service.Image)
			if strings.HasSuffix(strings.ToLower(service.Image), ":latest") || !strings.Contains(service.Image, ":") {
				result.Warnings = append(result.Warnings, fmt.Sprintf("A imagem do serviço %q não fixa uma versão.", name))
			}
		}
		if service.ContainerName != "" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("O serviço %q usa container_name, que não é recomendado para stacks.", name))
		}
		if service.Restart != "" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("O serviço %q usa restart; prefira deploy.restart_policy no Swarm.", name))
		}
		if len(service.DependsOn) > 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("depends_on do serviço %q não tem a mesma semântica no Swarm.", name))
		}
		for _, port := range service.Ports {
			result.Summary.Ports = append(result.Summary.Ports, fmt.Sprintf("%s:%d", port.Published, port.Target))
		}
	}
	result.Summary.EnvironmentVariables = uniqueVariables(result.Summary.EnvironmentVariables)
	for name := range project.Volumes {
		result.Summary.Volumes = append(result.Summary.Volumes, name)
	}
	for name := range project.Networks {
		result.Summary.Networks = append(result.Summary.Networks, name)
	}
	result.Valid = len(result.Errors) == 0
	return result
}

func extractAllVariables(summary *Summary, doc *yaml.Node) {
	if doc == nil {
		return
	}
	root := doc
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "services" || root.Content[i+1].Kind != yaml.MappingNode {
			continue
		}
		services := root.Content[i+1]
		for j := 0; j+1 < len(services.Content); j += 2 {
			extractVariables(summary, doc, services.Content[j].Value)
		}
	}
}

func yamlValue(raw string) *yaml.Node {
	var doc yaml.Node
	if yaml.Unmarshal([]byte(raw), &doc) != nil {
		return nil
	}
	return &doc
}

func extractVariables(summary *Summary, doc *yaml.Node, serviceName string) {
	if doc == nil {
		return
	}
	var walk func(*yaml.Node, bool)
	walk = func(node *yaml.Node, inEnvironment bool) {
		if node == nil {
			return
		}
		if node.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(node.Content); i += 2 {
				key, value := node.Content[i], node.Content[i+1]
				if inEnvironment {
					addEnvironmentVariable(summary, key.Value, serviceName)
				}
				walk(value, inEnvironment || key.Value == "environment")
			}
			return
		}
		if node.Kind == yaml.SequenceNode {
			for _, child := range node.Content {
				if inEnvironment && child.Kind == yaml.ScalarNode {
					name := child.Value
					if separator := strings.IndexByte(name, '='); separator >= 0 {
						name = name[:separator]
					}
					addEnvironmentVariable(summary, name, serviceName)
				}
				walk(child, inEnvironment)
			}
			return
		}
		if !inEnvironment || node.Kind != yaml.ScalarNode {
			return
		}
		for _, match := range variableReference.FindAllStringSubmatch(node.Value, -1) {
			name, operator, defaultValue := match[1], match[2], match[3]
			item := EnvironmentVariable{Name: name, DefaultValue: defaultValue, Required: operator == "" || strings.Contains(operator, "?"), Secret: isSecretName(name), Services: []string{serviceName}}
			summary.EnvironmentVariables = append(summary.EnvironmentVariables, item)
		}
	}
	services := doc
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		services = doc.Content[0]
	}
	if services.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(services.Content); i += 2 {
			if services.Content[i].Value != "services" {
				continue
			}
			serviceMap := services.Content[i+1]
			for j := 0; j+1 < len(serviceMap.Content); j += 2 {
				if serviceMap.Content[j].Value == serviceName {
					walk(serviceMap.Content[j+1], false)
					return
				}
			}
		}
	}
}

func addEnvironmentVariable(summary *Summary, name, serviceName string) {
	if !environmentKey.MatchString(name) {
		return
	}
	summary.EnvironmentVariables = append(summary.EnvironmentVariables, EnvironmentVariable{
		Name: name, Secret: isSecretName(name), Services: []string{serviceName},
	})
}

func isSecretName(name string) bool {
	upper := strings.ToUpper(name)
	for _, part := range []string{"PASSWORD", "SECRET", "TOKEN", "PRIVATE", "CREDENTIAL", "API_KEY"} {
		if strings.Contains(upper, part) {
			return true
		}
	}
	return false
}

func uniqueVariables(items []EnvironmentVariable) []EnvironmentVariable {
	byName := map[string]EnvironmentVariable{}
	for _, item := range items {
		current, exists := byName[item.Name]
		if !exists {
			byName[item.Name] = item
			continue
		}
		if current.DefaultValue == "" {
			current.DefaultValue = item.DefaultValue
		}
		current.Required = current.Required || item.Required
		current.Secret = current.Secret || item.Secret
		for _, service := range item.Services {
			if !contains(current.Services, service) {
				current.Services = append(current.Services, service)
			}
		}
		byName[item.Name] = current
	}
	out := make([]EnvironmentVariable, 0, len(byName))
	for _, item := range byName {
		sort.Strings(item.Services)
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func contains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
