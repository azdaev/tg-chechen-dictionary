package business

import (
	"chetoru/internal/models"

	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// doshamHTTPClient is shared across all dosham API calls so connections are
// reused and every request is bounded by a timeout. Without a timeout a hung
// API would block request goroutines indefinitely.
var doshamHTTPClient = &http.Client{Timeout: 15 * time.Second}

// maxRetryRequests caps how many respelling retries may be in flight against
// dosham at once, process-wide. cascadeBudget caps how long one cascade may
// spend waiting for them — a pool without a deadline just converts a burst into
// a long queue when the upstream is slow.
const (
	maxRetryRequests = 8
	cascadeBudget    = 20 * time.Second
)

// retrySlots is a pool retries take from and primary lookups do not. Primary
// lookups are already bounded by maxConcurrentUpdates on the handler side, and
// one shared pool would be a mistake: a cascade of somebody else's respellings
// could take every slot ahead of a fresh first lookup and make an ordinary
// search wait on a stranger's typo. Two disjoint pools make that impossible.
var retrySlots = make(chan struct{}, maxRetryRequests)

// fetchRetryFromAPI runs a respelling lookup under the retry budget, giving up
// its place the moment ctx is cancelled — which is what the sibling that found
// an exact match does.
func (b *Business) fetchRetryFromAPI(ctx context.Context, word string) ([]models.TranslationPairs, error) {
	select {
	case retrySlots <- struct{}{}:
		defer func() { <-retrySlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return b.fetchFromAPI(ctx, word)
}

// doDoshamQuery runs a GraphQL query against the dosham API and decodes the
// response into out. It centralizes the endpoint, timeout, and JSON plumbing
// shared by the translation, /random, and /quiz features.
func doDoshamQuery(ctx context.Context, query string, variables map[string]any, out any) error {
	body := map[string]any{"query": query}
	if variables != nil {
		body["variables"] = variables
	}

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", doshamAPIURL(), bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := doshamHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("dosham API: status %d", resp.StatusCode)
	}

	// A failed GraphQL query still decodes cleanly into an empty result, which
	// callers would mistake for a real "no results" answer (and cache it).
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var gqlErrs struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &gqlErrs); err != nil {
		return err
	}
	if len(gqlErrs.Errors) > 0 {
		return fmt.Errorf("dosham API: %s", gqlErrs.Errors[0].Message)
	}

	return json.Unmarshal(raw, out)
}
