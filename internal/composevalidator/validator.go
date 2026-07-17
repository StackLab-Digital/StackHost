package composevalidator

import (
	"context"
	"fmt"
	"strings"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
)

type Result struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
	Summary  Summary  `json:"summary"`
}

type Summary struct {
	Services []string `json:"services"`
	Images   []string `json:"images"`
	Ports    []string `json:"ports"`
	Volumes  []string `json:"volumes"`
	Networks []string `json:"networks"`
}

func Validate(yaml string) Result {
	result := Result{Summary: Summary{Services: []string{}, Images: []string{}, Ports: []string{}, Volumes: []string{}, Networks: []string{}}}
	if strings.TrimSpace(yaml) == "" {
		result.Errors = append(result.Errors, "Cole um arquivo Docker Compose para continuar.")
		return result
	}
	config := types.ConfigDetails{WorkingDir: ".", ConfigFiles: []types.ConfigFile{{Filename: "stackhost.yml", Content: []byte(yaml)}}, Environment: types.Mapping{"COMPOSE_PROJECT_NAME": "stackhost"}}
	project, err := loader.LoadWithContext(context.Background(), config, func(options *loader.Options) {
		options.SetProjectName("stackhost", true)
	})
	if err != nil {
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
	for name := range project.Volumes {
		result.Summary.Volumes = append(result.Summary.Volumes, name)
	}
	for name := range project.Networks {
		result.Summary.Networks = append(result.Summary.Networks, name)
	}
	result.Valid = len(result.Errors) == 0
	return result
}
