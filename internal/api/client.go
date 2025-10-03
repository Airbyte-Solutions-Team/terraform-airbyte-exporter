package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client represents an HTTP API client
type Client struct {
	baseURL      string
	apiKey       string
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

type ApiKeyResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// NewClient creates a new API client
func NewClient(baseURL, clientID string, clientSecret string) *Client {
	return &Client{
		baseURL:      baseURL,
		apiKey:       "",
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Get performs a GET request to the specified endpoint
func (c *Client) Get(endpoint string) ([]byte, error) {
	if c.apiKey == "" && c.clientID != "" && c.clientSecret != "" {
		apiKey, err := c.generateApiKey()
		if err != nil {
			return nil, fmt.Errorf("failed to generate API key: %w", err)
		}
		c.apiKey = apiKey
	}

	req, err := http.NewRequest("GET", c.baseURL+endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication if API key is provided
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("the Airbyte API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return body, nil
}

func (c *Client) generateApiKey() (string, error) {
	req, err := http.NewRequest("POST", c.baseURL+"/v1/applications/token", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add JSON body with client_id and client_secret
	payload := map[string]string{
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth payload: %w", err)
	}
	req.Body = io.NopCloser(bytes.NewReader(payloadBytes))
	req.ContentLength = int64(len(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("the Airbyte API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}
	var apiKeyResp ApiKeyResponse
	err = json.Unmarshal(body, &apiKeyResp)
	if err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return apiKeyResp.AccessToken, nil
}
