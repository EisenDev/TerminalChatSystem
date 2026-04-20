package auth

import (
	"fmt"
	"regexp"
	"strings"
)

var handlePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{2,32}$`)

func NormalizeHandle(handle string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(handle))
	if !handlePattern.MatchString(normalized) {
		return "", fmt.Errorf("handle must be 2-32 chars and contain only letters, numbers, '_' or '-'")
	}
	return normalized, nil
}
