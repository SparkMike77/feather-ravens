package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

var ingestHTTPClient = &http.Client{Timeout: 10 * time.Second}

// postCandidate sends one matched, full-text-fetched candidate to Feather's ingest endpoint.
// Contract (not yet implemented on the Feather side - see the project README): POST ingestURL
// with a single Candidate as the JSON body; any non-2xx response is treated as delivery failure.
func postCandidate(ingestURL string, candidate Candidate) error {
	body, err := json.Marshal(candidate)
	if err != nil {
		return fmt.Errorf("encoding candidate: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, ingestURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := ingestHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("posting to %s: %w", ingestURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("ingest endpoint returned %s", resp.Status)
	}
	return nil
}
