package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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

// PaginatedResponse represents a paginated API response
type PaginatedResponse struct {
	Data     json.RawMessage `json:"data"`
	Next     string          `json:"next,omitempty"`
	Previous string          `json:"previous,omitempty"`
}

// Get performs a GET request to the specified endpoint with pagination support
// It automatically follows "next" links to retrieve all pages and merges the results
func (c *Client) Get(endpoint string, workspaceId *string) ([]byte, error) {
	if c.apiKey == "" && c.clientID != "" && c.clientSecret != "" {
		apiKey, err := c.generateApiKey()
		if err != nil {
			return nil, fmt.Errorf("failed to generate API key: %w", err)
		}
		c.apiKey = apiKey
	}

	// Build initial URL with limit parameter
	apiUrl, err := url.JoinPath(c.baseURL, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to join URL paths: %w", err)
	}

	u, err := url.Parse(apiUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	q := u.Query()
	// Set limit to 100 for better performance
	q.Set("limit", "100")
	// Add workspaceId as query parameter if provided
	if workspaceId != nil && *workspaceId != "" {
		q.Set("workspaceIds", *workspaceId)
	}
	u.RawQuery = q.Encode()
	apiUrl = u.String()

	// Collect all data across pages
	var allData []json.RawMessage
	currentURL := apiUrl
	pageCount := 0

	for {
		pageCount++
		fmt.Fprintf(os.Stderr, "Fetching page %d from: %s\n", pageCount, currentURL)

		body, err := c.fetchPage(currentURL)
		if err != nil {
			return nil, err
		}

		// Try to parse as paginated response
		var paginatedResp PaginatedResponse
		if err := json.Unmarshal(body, &paginatedResp); err != nil {
			// Not a paginated response, return the body as-is
			fmt.Fprintf(os.Stderr, "Response is not paginated, returning single page\n")
			return body, nil
		}

		// Add this page's data to our collection
		allData = append(allData, paginatedResp.Data)

		// Check if there's a next page
		if paginatedResp.Next == "" {
			fmt.Fprintf(os.Stderr, "Reached last page, total pages fetched: %d\n", pageCount)
			break
		}

		// Follow the next link
		currentURL = paginatedResp.Next
	}

	// Merge all pages into a single response
	mergedData, err := c.mergePages(allData)
	if err != nil {
		return nil, fmt.Errorf("failed to merge paginated results: %w", err)
	}

	// Return as a response with "data" field
	finalResponse := map[string]interface{}{
		"data": mergedData,
	}
	return json.Marshal(finalResponse)
}

// fetchPage performs a single HTTP GET request
func (c *Client) fetchPage(urlStr string) ([]byte, error) {
	req, err := http.NewRequest("GET", urlStr, nil)
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

// mergePages merges multiple pages of data into a single array
func (c *Client) mergePages(pages []json.RawMessage) ([]interface{}, error) {
	var merged []interface{}

	for _, page := range pages {
		var pageData []interface{}
		if err := json.Unmarshal(page, &pageData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal page data: %w", err)
		}
		merged = append(merged, pageData...)
	}

	return merged, nil
}

// GetWorkspaces fetches all workspaces from the Airbyte API
func (c *Client) GetWorkspaces() ([]byte, error) {
	return c.Get("/v1/workspaces", nil)
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
