package net

import (
	"strings"
	"testing"
)

func TestBuildStatsMessage_CacheSection(t *testing.T) {
	msg := buildStatsMessage(statsData{month: 6, year: 2026, cacheHits: 90, cacheMisses: 10})
	if !strings.Contains(msg, "Кэш переводов") || !strings.Contains(msg, "(90%)") {
		t.Fatalf("cache section missing or wrong:\n%s", msg)
	}

	// Before any lookups the section is omitted rather than showing 0/0.
	if msg := buildStatsMessage(statsData{month: 6, year: 2026}); strings.Contains(msg, "Кэш переводов") {
		t.Fatalf("cache section must be omitted with no lookups:\n%s", msg)
	}
}
