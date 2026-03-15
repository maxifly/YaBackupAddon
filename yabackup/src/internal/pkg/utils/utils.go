package utils

import "strings"

func ConcatAndTruncateUnicode(items []string, sep string, maxLen int) string {
	result := strings.Join(items, sep)
	runes := []rune(result)

	if len(runes) > maxLen {
		runes = runes[:maxLen]
	}

	return string(runes)
}
