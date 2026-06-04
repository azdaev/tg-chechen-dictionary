package business

import (
	"chetoru/internal/models"
	"context"
	"net/http"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestWarmWordPool_FillsFromAPI(t *testing.T) {
	stubDoshamAPI(t, http.StatusOK, `{"data":{"randomEntries":[
		{"content":"дитт","type":"WORD","translations":[{"content":"дерево","languageCode":"ru"}]},
		{"content":"цӀа","type":"WORD","translations":[{"content":"дом","languageCode":"ru"}]}
	]}}`)
	b := &Business{log: logrus.New()}

	b.WarmWordPool(context.Background())
	if got := b.pool.size(); got != 2 {
		t.Fatalf("pool size after warmup = %d, want 2", got)
	}

	// A failed warmup logs and leaves the pool empty, never panics.
	b2 := &Business{log: logrus.New()}
	stubDoshamAPI(t, http.StatusInternalServerError, ``)
	b2.WarmWordPool(context.Background())
	if got := b2.pool.size(); got != 0 {
		t.Fatalf("pool size after failed warmup = %d, want 0", got)
	}
}

func TestWordPool_InsertDedupes(t *testing.T) {
	var p wordPool
	p.insert(models.RandomWord{Chechen: "дитт", Russian: "дерево"})
	p.insert(models.RandomWord{Chechen: "Дитт", Russian: "ствол"}) // duplicate word
	p.insert(models.RandomWord{Chechen: "хен", Russian: "ДЕРЕВО"}) // duplicate meaning
	p.insert(models.RandomWord{Chechen: "цӀа", Russian: "дом"})

	if got := p.size(); got != 2 {
		t.Fatalf("pool size = %d, want 2 (duplicates on either side rejected)", got)
	}
}

func TestWordPool_DrawConsumes(t *testing.T) {
	var p wordPool
	p.insert(models.RandomWord{Chechen: "а", Russian: "б"})
	p.insert(models.RandomWord{Chechen: "в", Russian: "г"})
	p.insert(models.RandomWord{Chechen: "д", Russian: "е"})

	got := p.draw(2)
	if len(got) != 2 || p.size() != 1 {
		t.Fatalf("draw(2) = %d items, %d left; want 2 and 1", len(got), p.size())
	}
	if got[0].Chechen == got[1].Chechen {
		t.Fatalf("drew the same item twice: %+v", got)
	}

	// Over-draw returns what's left, never errors.
	if rest := p.draw(5); len(rest) != 1 || p.size() != 0 {
		t.Fatalf("over-draw = %d items, %d left; want 1 and 0", len(rest), p.size())
	}
}

func TestWordPool_RefillGate(t *testing.T) {
	var p wordPool
	p.insert(models.RandomWord{Chechen: "а", Russian: "б"})

	if !p.startRefill(5) {
		t.Fatal("below threshold and idle: refill should start")
	}
	if p.startRefill(5) {
		t.Fatal("refill already running: second start must be refused")
	}
	p.endRefill()
	if !p.startRefill(5) {
		t.Fatal("after endRefill a new refill should start")
	}
	p.endRefill()

	for i := range 5 {
		p.insert(models.RandomWord{Chechen: string(rune('а' + i + 1)), Russian: string(rune('я' - i))})
	}
	if p.startRefill(5) {
		t.Fatal("at/above threshold: refill should not start")
	}
}
