package composevalidator

import "testing"

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
