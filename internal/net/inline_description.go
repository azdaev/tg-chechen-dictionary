package net

import (
	"chetoru/pkg/tools"
	"fmt"
	"strings"
)

func inlineDescription(formatted, original string) string {
	lines := strings.Split(formatted, "\n")
	cleanLines := make([]string, 0, len(lines))
	for _, line := range lines {
		cleanLines = append(cleanLines, tools.Clean(line))
	}
	clean := strings.Join(cleanLines, "\n")
	if stripped := strings.TrimPrefix(clean, tools.Clean(original)+" — "); stripped != clean {
		return strings.ReplaceAll(stripped, "\n", " · ")
	}

	if len(cleanLines) >= 2 && isDirectionLine(cleanLines[1]) {
		return inlineCardSummary(cleanLines)
	}

	if len(cleanLines) >= 2 && isLanguageHeaderLine(cleanLines[0]) && isLanguageHeaderLine(cleanLines[1]) {
		head := strings.TrimSpace(afterColon(cleanLines[1]))
		rest := trimBlankLines(cleanLines[2:])
		if head != "" {
			rest = append([]string{head}, rest...)
		}
		return strings.Join(rest, " · ")
	}
	return strings.ReplaceAll(clean, "\n", " · ")
}

func inlineCardSummary(lines []string) string {
	translations := collectSectionLines(lines, "Перевод", "Переводы")
	examples := collectSectionLines(lines, "Примеры")
	parts := make([]string, 0, 3)
	if len(translations) > 0 {
		parts = append(parts, stripBullet(translations[0]))
	}
	if len(examples) > 0 {
		parts = append(parts, fmt.Sprintf("%d прим.", len(examples)))
	}
	if len(parts) == 0 && len(lines) > 0 {
		return lines[0]
	}
	return strings.Join(parts, " · ")
}

func collectSectionLines(lines []string, names ...string) []string {
	nameSet := make(map[string]bool, len(names))
	for _, name := range names {
		nameSet[name] = true
	}
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if !nameSet[line] {
			continue
		}
		return trimBlankLinesUntilSection(lines[i+1:])
	}
	return nil
}

func trimBlankLinesUntilSection(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(out) == 0 {
				continue
			}
			break
		}
		if !strings.HasPrefix(line, "•") {
			break
		}
		out = append(out, line)
	}
	return out
}

func stripBullet(line string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "•"))
}

func isDirectionLine(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "с русского на ") || strings.HasPrefix(line, "с чеченского на ")
}

func isLanguageHeaderLine(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "Русский:") || strings.HasPrefix(line, "Чеченский:")
}

func afterColon(line string) string {
	_, rest, ok := strings.Cut(line, ":")
	if !ok {
		return line
	}
	return strings.TrimSpace(rest)
}

func trimBlankLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
