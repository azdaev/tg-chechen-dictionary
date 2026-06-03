package business

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// doshamHTTPClient is shared across all dosham API calls so connections are
// reused and every request is bounded by a timeout. Without a timeout a hung
// API would block request goroutines indefinitely.
var doshamHTTPClient = &http.Client{Timeout: 15 * time.Second}

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

	return json.NewDecoder(resp.Body).Decode(out)
}
