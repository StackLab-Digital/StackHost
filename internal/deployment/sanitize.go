package deployment

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const DefaultOutputLimit = 64 * 1024

var ansiEscape = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\a]*(?:\a|\x1b\\))`)

func SanitizeOutput(output string, sensitiveValues []string, limit int) string {
	output = strings.ToValidUTF8(output, "�")
	output = ansiEscape.ReplaceAllString(output, "")
	output = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r >= 0x20 {
			return r
		}
		return -1
	}, output)

	values := append([]string(nil), sensitiveValues...)
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	for _, value := range values {
		if value != "" {
			output = strings.ReplaceAll(output, value, "[REDACTED]")
		}
	}
	if limit <= 0 {
		limit = DefaultOutputLimit
	}
	if len(output) <= limit {
		return output
	}
	marker := "\n[output truncated]"
	if limit <= len(marker) {
		return marker[:limit]
	}
	output = output[:limit-len(marker)]
	for !utf8.ValidString(output) {
		output = output[:len(output)-1]
	}
	return output + marker
}
