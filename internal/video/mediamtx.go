package video

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type MediaMtxClient interface {
	AddPath(ctx context.Context, name string, rtspURL string) error
	DeletePath(ctx context.Context, name string) error
	ListActivePaths(ctx context.Context) ([]MediaMtxPathItem, error)
}

type mediaMtxClient struct {
	apiURL     string
	httpClient *http.Client
}

func NewMediaMtxClient(apiURL string) MediaMtxClient {
	return &mediaMtxClient{
		apiURL: apiURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *mediaMtxClient) AddPath(ctx context.Context, name string, rtspURL string) error {
	url := fmt.Sprintf("%s/v3/config/paths/add/%s", c.apiURL, name)
	
	payload := map[string]interface{}{
		"source":         rtspURL,
		"sourceOnDemand": true,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal add path payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute add path request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status code from MediaMTX add path: %d", resp.StatusCode)
	}

	return nil
}

func (c *mediaMtxClient) DeletePath(ctx context.Context, name string) error {
	url := fmt.Sprintf("%s/v3/config/paths/delete/%s", c.apiURL, name)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute delete path request: %w", err)
	}
	defer resp.Body.Close()

	// 404 is acceptable (path wasn't configured)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("unexpected status code from MediaMTX delete path: %d", resp.StatusCode)
	}

	return nil
}

func (c *mediaMtxClient) ListActivePaths(ctx context.Context) ([]MediaMtxPathItem, error) {
	url := fmt.Sprintf("%s/v3/paths/list", c.apiURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute list active paths request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from MediaMTX list active paths: %d", resp.StatusCode)
	}

	var res MediaMtxActivePathsResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode list active paths response: %w", err)
	}

	return res.Items, nil
}
