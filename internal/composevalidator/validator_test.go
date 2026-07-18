package composevalidator

import (
	"strings"
	"testing"
)

func TestValidateComposeProducesSafeSummary(t *testing.T) {
	result := Validate("services:\n  web:\n    image: nginx:1.27-alpine\n    ports:\n      - \"8080:80\"\n")
	if !result.Valid || len(result.Summary.Services) != 1 || result.Summary.Images[0] != "nginx:1.27-alpine" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestValidateComposeReportsUnsupportedBuild(t *testing.T) {
	result := Validate("services:\n  web:\n    build: .\n")
	if result.Valid || len(result.Errors) == 0 {
		t.Fatalf("expected build validation error: %+v", result)
	}
}

func TestValidateComposeExtractsVariables(t *testing.T) {
	result := Validate("services:\n  web:\n    image: nginx:1.27\n    environment:\n      DATABASE_URL: ${DATABASE_URL}\n      REDIS_URL: ${REDIS_URL:-redis://redis:6379}\n      API_TOKEN: ${API_TOKEN:?Informe}\n  worker:\n    image: worker:1.0\n    environment:\n      DATABASE_URL: ${DATABASE_URL}\n")
	if len(result.Summary.EnvironmentVariables) != 3 {
		t.Fatalf("unexpected variables: %+v", result.Summary.EnvironmentVariables)
	}
	for _, item := range result.Summary.EnvironmentVariables {
		if item.Name == "DATABASE_URL" && len(item.Services) != 2 {
			t.Fatalf("expected service aggregation: %+v", item)
		}
		if item.Name == "API_TOKEN" && (!item.Secret || !item.Required) {
			t.Fatalf("expected secret required variable: %+v", item)
		}
	}
}

func TestValidateComposeUsesProvidedEnvironment(t *testing.T) {
	result := ValidateWithEnvironment("services:\n  web:\n    image: nginx:1.27\n    environment:\n      DATABASE_URL: ${DATABASE_URL:?required}\n", map[string]string{"DATABASE_URL": "postgres://db"})
	if !result.Valid {
		t.Fatalf("expected configured variable to validate: %+v", result)
	}
}

func TestValidateComposeExtractsLiteralEnvironmentKeys(t *testing.T) {
	result := Validate("services:\n  web:\n    image: nginx:1.27\n    environment:\n      APP_ENV: production\n      API_TOKEN: value\n  worker:\n    image: worker:1.0\n    environment:\n      - QUEUE_NAME=default\n")
	if len(result.Summary.EnvironmentVariables) != 3 {
		t.Fatalf("unexpected variables: %+v", result.Summary.EnvironmentVariables)
	}
	for _, item := range result.Summary.EnvironmentVariables {
		if item.Name == "API_TOKEN" && !item.Secret {
			t.Fatalf("expected literal secret heuristic: %+v", item)
		}
	}
}

func TestValidateComposeAllowsNamedVolumes(t *testing.T) {
	result := Validate("services:\n  db:\n    image: postgres:16\n    volumes:\n      - data:/var/lib/postgresql/data\nvolumes:\n  data:\n")
	if !result.Valid || len(result.Summary.Volumes) != 1 || result.Summary.Volumes[0] != "data" {
		t.Fatalf("expected named volume to remain supported: %+v", result)
	}
}

func TestValidateComposeAllowsSameFileExtends(t *testing.T) {
	result := Validate("services:\n  base:\n    image: nginx:1.27\n  web:\n    extends:\n      service: base\n")
	if !result.Valid {
		t.Fatalf("expected same-file extends to remain supported: %+v", result)
	}
}

func TestValidateComposeRejectsHostBoundaryFeatures(t *testing.T) {
	tests := []struct {
		name, compose, errorPart string
	}{
		{"include", "include:\n  - /etc/compose.yml\nservices:\n  web:\n    image: nginx:1.27\n", "include"},
		{"extends file", "services:\n  web:\n    image: nginx:1.27\n    extends:\n      file: /etc/compose.yml\n      service: base\n", "extends com arquivo"},
		{"environment file", "services:\n  web:\n    image: nginx:1.27\n    env_file: /etc/environment\n", "env_file"},
		{"label file", "services:\n  web:\n    image: nginx:1.27\n    label_file: /etc/labels\n", "label_file"},
		{"bind mount", "services:\n  web:\n    image: nginx:1.27\n    volumes:\n      - /etc:/host:ro\n", "caminhos do host"},
		{"long bind mount", "services:\n  web:\n    image: nginx:1.27\n    volumes:\n      - type: bind\n        source: ./data\n        target: /data\n", "caminhos do host"},
		{"docker socket", "services:\n  web:\n    image: nginx:1.27\n    volumes:\n      - /var/run/docker.sock:/var/run/docker.sock\n", "socket do Docker"},
		{"privileged", "services:\n  web:\n    image: nginx:1.27\n    privileged: true\n", "privileged"},
		{"host network", "services:\n  web:\n    image: nginx:1.27\n    network_mode: host\n", "network_mode do host"},
		{"host pid", "services:\n  web:\n    image: nginx:1.27\n    pid: host\n", "pid do host"},
		{"host ipc", "services:\n  web:\n    image: nginx:1.27\n    ipc: host\n", "ipc do host"},
		{"devices", "services:\n  web:\n    image: nginx:1.27\n    devices:\n      - /dev/kvm:/dev/kvm\n", "devices do host"},
		{"config file", "services:\n  web:\n    image: nginx:1.27\n    configs:\n      - app\nconfigs:\n  app:\n    file: /etc/hostname\n", "configs"},
		{"secret file", "services:\n  web:\n    image: nginx:1.27\n    secrets:\n      - app\nsecrets:\n  app:\n    file: /etc/hostname\n", "secrets"},
		{"named bind volume", "services:\n  web:\n    image: nginx:1.27\n    volumes:\n      - data:/data\nvolumes:\n  data:\n    driver: local\n    driver_opts:\n      type: none\n      o: bind\n      device: /etc\n", "volume \"data\""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Validate(test.compose)
			if result.Valid || !strings.Contains(strings.Join(result.Errors, " "), test.errorPart) {
				t.Fatalf("expected %q rejection: %+v", test.errorPart, result)
			}
		})
	}
}

func TestValidateComposeRejectsMergedHostFileReference(t *testing.T) {
	result := Validate("x-host: &host\n  env_file: /etc/environment\nservices:\n  web:\n    image: nginx:1.27\n    <<: *host\n")
	if result.Valid || !strings.Contains(strings.Join(result.Errors, " "), "env_file") {
		t.Fatalf("expected merged env_file to be rejected before loading: %+v", result)
	}
}
