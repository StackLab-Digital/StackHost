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
