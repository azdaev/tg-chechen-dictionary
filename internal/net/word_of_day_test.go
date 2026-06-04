package net

import (
	"chetoru/internal/models"
	"testing"
	"time"
)

func TestPickFresh(t *testing.T) {
	words := []string{"дитт", "цӏа", "ӏаж"}
	i := 0
	draw := func() *models.RandomWord {
		if i >= len(words) {
			return nil
		}
		w := &models.RandomWord{Chechen: words[i]}
		i++
		return w
	}

	// First two were sent recently (case-insensitively); the third wins.
	got := pickFresh([]string{"Дитт", "ЦӀа"}, draw)
	if got == nil || got.Chechen != "ӏаж" {
		t.Fatalf("pickFresh = %+v, want ӏаж", got)
	}

	// Everything is recent: settle for the first draw, never skip the day.
	i = 0
	got = pickFresh([]string{"дитт", "цӏа", "ӏаж"}, draw)
	if got == nil || got.Chechen != "дитт" {
		t.Fatalf("pickFresh fallback = %+v, want дитт", got)
	}

	// No words at all.
	if got := pickFresh(nil, func() *models.RandomWord { return nil }); got != nil {
		t.Fatalf("pickFresh with no draws = %+v, want nil", got)
	}
}

func TestWordOfDayDue(t *testing.T) {
	morning := time.Date(2026, 6, 4, 8, 0, 0, 0, time.UTC)
	afternoon := time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)

	if wordOfDayDue(morning, "2026-06-03", 9) {
		t.Error("before the send hour nothing is due yet")
	}
	if !wordOfDayDue(afternoon, "2026-06-03", 9) {
		t.Error("past the send hour with yesterday's record: due")
	}
	if wordOfDayDue(afternoon, "2026-06-04", 9) {
		t.Error("already sent today: not due")
	}
}

func TestNextWordOfDayTime(t *testing.T) {
	loc := time.UTC

	cases := []struct {
		name     string
		now      time.Time
		hour     int
		wantDay  int
		wantHour int
	}{
		{
			name:     "before target hour -> later today",
			now:      time.Date(2026, 6, 3, 7, 30, 0, 0, loc),
			hour:     9,
			wantDay:  3,
			wantHour: 9,
		},
		{
			name:     "after target hour -> tomorrow",
			now:      time.Date(2026, 6, 3, 10, 0, 0, 0, loc),
			hour:     9,
			wantDay:  4,
			wantHour: 9,
		},
		{
			name:     "exactly at target hour -> tomorrow (not now)",
			now:      time.Date(2026, 6, 3, 9, 0, 0, 0, loc),
			hour:     9,
			wantDay:  4,
			wantHour: 9,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nextWordOfDayTime(tc.now, tc.hour)
			if got.Day() != tc.wantDay || got.Hour() != tc.wantHour || got.Minute() != 0 {
				t.Errorf("nextWordOfDayTime(%v, %d) = %v, want day=%d hour=%d minute=0",
					tc.now, tc.hour, got, tc.wantDay, tc.wantHour)
			}
			if !got.After(tc.now) {
				t.Errorf("next time %v is not after now %v", got, tc.now)
			}
		})
	}
}
