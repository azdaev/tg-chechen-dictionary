package business

import (
	"chetoru/internal/models"
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
)

const (
	poolFetchSize   = 40
	poolRefillBelow = 12
	poolFillTries   = 3
)

// wordPool holds prefetched clean Chechen→Russian pairs so /random and /quiz
// answer instantly instead of paying a live randomEntries call each time.
// Words are consumed on draw and the pool refills in the background. All
// pooled pairs are mutually distinct on both sides, so any draw is directly
// usable as quiz options.
type wordPool struct {
	mu        sync.Mutex
	words     []models.RandomWord
	refilling bool
}

func (p *wordPool) size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.words)
}

// insert adds a pair unless either side duplicates a pooled entry.
func (p *wordPool) insert(w models.RandomWord) {
	wordKey := strings.ToLower(w.Chechen)
	meaningKey := strings.ToLower(w.Russian)

	p.mu.Lock()
	defer p.mu.Unlock()
	for _, have := range p.words {
		if strings.ToLower(have.Chechen) == wordKey || strings.ToLower(have.Russian) == meaningKey {
			return
		}
	}
	p.words = append(p.words, w)
}

// draw removes and returns up to n random pairs.
func (p *wordPool) draw(n int) []models.RandomWord {
	p.mu.Lock()
	defer p.mu.Unlock()
	if n > len(p.words) {
		n = len(p.words)
	}
	out := make([]models.RandomWord, 0, n)
	for range n {
		i := rand.IntN(len(p.words))
		out = append(out, p.words[i])
		p.words[i] = p.words[len(p.words)-1]
		p.words = p.words[:len(p.words)-1]
	}
	return out
}

// startRefill marks the pool as refilling when it is below threshold and no
// refill is already running. The caller must call endRefill when done.
func (p *wordPool) startRefill(threshold int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.refilling || len(p.words) >= threshold {
		return false
	}
	p.refilling = true
	return true
}

func (p *wordPool) endRefill() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refilling = false
}

// fillPool fetches one random batch and pools its clean WORD pairs.
func (b *Business) fillPool(ctx context.Context) error {
	entries, err := b.fetchRandomEntries(ctx, poolFetchSize)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Type != "WORD" {
			continue
		}
		if w := orientEntry(entry); w != nil && isLearnableWord(w.Chechen) && isCleanMeaning(w.Russian) {
			b.pool.insert(*w)
		}
	}
	return nil
}

// WarmWordPool fills the pool ahead of demand, so the first /random or /quiz
// after a deploy doesn't pay the API round trips.
func (b *Business) WarmWordPool(ctx context.Context) {
	if !b.pool.startRefill(poolRefillBelow) {
		return
	}
	defer b.pool.endRefill()
	if err := b.fillPool(ctx); err != nil {
		b.log.Printf("word pool warmup failed: %v\n", err)
	}
}

// randomCleanWords draws n mutually distinct pairs, filling the pool
// synchronously only when it cannot cover the request, and kicks off a
// background refill when the pool runs low.
func (b *Business) randomCleanWords(ctx context.Context, n int) ([]models.RandomWord, error) {
	var lastErr error
	for range poolFillTries {
		if b.pool.size() >= n {
			break
		}
		if err := b.fillPool(ctx); err != nil {
			lastErr = err
		}
	}

	words := b.pool.draw(n)
	if len(words) < n {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("not enough clean random words: have %d, need %d", len(words), n)
	}

	if b.pool.startRefill(poolRefillBelow) {
		go func() {
			defer b.pool.endRefill()
			if err := b.fillPool(context.Background()); err != nil {
				b.log.Printf("word pool refill failed: %v\n", err)
			}
		}()
	}
	return words, nil
}
