package templates

import "strings"

func TrimSpace(content string) string {
	return strings.TrimSpace(content) + "\n"
}
