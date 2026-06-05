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

func TestBuildStatsMessage_EngagementLines(t *testing.T) {
	msg := buildStatsMessage(statsData{month: 6, year: 2026, wotdChats: 4, activeStreaks: 12})
	if !strings.Contains(msg, "Групп на «Слове дня»: <b>4</b>") || !strings.Contains(msg, "Активных серий: <b>12</b>") {
		t.Fatalf("engagement lines missing:\n%s", msg)
	}

	// Zero values hide the optional lines instead of reporting zeros.
	msg = buildStatsMessage(statsData{month: 6, year: 2026})
	if strings.Contains(msg, "Групп") || strings.Contains(msg, "серий") {
		t.Fatalf("optional lines must be hidden at zero:\n%s", msg)
	}
}

func TestBuildMeMessage(t *testing.T) {
	msg := buildMeMessage(1234, 8, 10, 3, 7, true)
	for _, want := range []string{"1 234", "8/10", "(80%)", "Серия: <b>3 дн.</b>", "Место в /top: <b>№7</b>", "Слово дня: <b>включено</b>"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q:\n%s", want, msg)
		}
	}

	// Unranked players (below the 3-answer bar) see no rank line.
	if msg := buildMeMessage(5, 1, 2, 0, 0, true); strings.Contains(msg, "Место") {
		t.Errorf("rank line must be hidden when unranked:\n%s", msg)
	}

	msg = buildMeMessage(0, 0, 0, 0, 0, false)
	if !strings.Contains(msg, "попробуйте /quiz") {
		t.Errorf("expected quiz nudge when never played:\n%s", msg)
	}
	if !strings.Contains(msg, "включить через /wotd") {
		t.Errorf("expected wotd nudge when unsubscribed:\n%s", msg)
	}
	if strings.Contains(msg, "Серия") {
		t.Errorf("streak line must be hidden below 2 days:\n%s", msg)
	}
}
