package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client represents an API client
type Client struct {
	baseURL      string
	clientID     string
	clientSecret string
	httpClient   *http.Client
	accessToken  string
	tokenExpiry  time.Time
}

// NewClient creates a new API client
func NewClient(baseURL, clientID, clientSecret string) *Client {
	return &Client{
		baseURL:      baseURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 60 * time.Second},
	}
}

// TokenRequest represents the request body for getting an access token
type TokenRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// TokenResponse represents the response from the token endpoint
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// getAccessToken retrieves an access token using the Airbyte API
func (c *Client) getAccessToken() error {
	// Check if we have a valid token that hasn't expired
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return nil
	}

	// Prepare the token request
	tokenReq := TokenRequest{
		ClientID:     c.clientID,
		ClientSecret: c.clientSecret,
	}

	// Convert to JSON
	jsonData, err := json.Marshal(tokenReq)
	if err != nil {
		return fmt.Errorf("failed to marshal token request: %w", err)
	}

	// Make the token request
	tokenURL := c.baseURL + "/v1/applications/token"
	req, err := http.NewRequest("POST", tokenURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("failed to decode token response: %w", err)
	}

	// Store the token and set expiry (tokens are valid for 3 minutes according to docs)
	c.accessToken = tokenResp.AccessToken
	c.tokenExpiry = time.Now().Add(3 * time.Minute)

	return nil
}

// Get makes a GET request to the specified endpoint, handling pagination automatically.
// All pages are merged into a single JSON response with all items in the "data" array.
func (c *Client) Get(endpoint string, workspaceID *string) ([]byte, error) {
	if err := c.getAccessToken(); err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	var allItems []json.RawMessage
	nextURL := c.baseURL + endpoint
	maxPages := 100

	for nextURL != "" && maxPages > 0 {
		maxPages--
		req, err := http.NewRequest("GET", nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.accessToken)

		// Only add query params on the first request; pagination URLs already include them
		if nextURL == c.baseURL+endpoint {
			q := req.URL.Query()
			if workspaceID != nil && *workspaceID != "" {
				q.Add("workspaceIds", *workspaceID)
			}
			q.Set("limit", "100")
			req.URL.RawQuery = q.Encode()
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to make request: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		var page struct {
			Data json.RawMessage `json:"data"`
			Next string          `json:"next"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return body, nil
		}

		if page.Data == nil {
			return body, nil
		}

		var items []json.RawMessage
		if err := json.Unmarshal(page.Data, &items); err != nil {
			return body, nil
		}

		allItems = append(allItems, items...)
		nextURL = page.Next
	}

	// Re-assemble into the expected {"data": [...]} envelope
	dataBytes, err := json.Marshal(allItems)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal combined results: %w", err)
	}
	result := fmt.Sprintf(`{"data":%s}`, string(dataBytes))
	return []byte(result), nil
}

// Post makes a POST request to the specified endpoint
func (c *Client) Post(endpoint string, data interface{}) ([]byte, error) {
	// Get a valid access token first
	if err := c.getAccessToken(); err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	url := c.baseURL + endpoint

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add Bearer token authentication
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// GetWorkspaces fetches all workspaces
func (c *Client) GetWorkspaces() ([]byte, error) {
	return c.Get("/v1/workspaces", nil)
}

// GetSources fetches sources for a workspace
func (c *Client) GetSources(workspaceID string) ([]byte, error) {
	return c.Get("/v1/sources", &workspaceID)
}

// GetDestinations fetches destinations for a workspace
func (c *Client) GetDestinations(workspaceID string) ([]byte, error) {
	return c.Get("/v1/destinations", &workspaceID)
}

// GetConnections fetches connections for a workspace
func (c *Client) GetConnections(workspaceID string) ([]byte, error) {
	return c.Get("/v1/connections", &workspaceID)
}

// GetConnectionByID fetches a specific connection by ID
func (c *Client) GetConnectionByID(connectionID string) ([]byte, error) {
	endpoint := fmt.Sprintf("/v1/connections/%s", connectionID)
	return c.Get(endpoint, nil)
}

// GetSourceByID fetches a specific source by ID
func (c *Client) GetSourceByID(sourceID string) ([]byte, error) {
	endpoint := fmt.Sprintf("/v1/sources/%s", sourceID)
	return c.Get(endpoint, nil)
}

// GetDestinationByID fetches a specific destination by ID
func (c *Client) GetDestinationByID(destinationID string) ([]byte, error) {
	endpoint := fmt.Sprintf("/v1/destinations/%s", destinationID)
	return c.Get(endpoint, nil)
}

// Patch makes a PATCH request to the specified endpoint
func (c *Client) Patch(endpoint string, data interface{}) ([]byte, error) {
	// Get a valid access token first
	if err := c.getAccessToken(); err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	url := c.baseURL + endpoint

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add Bearer token authentication
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// UpdateConnection updates a connection via the public API (PATCH /v1/connections/{id})
func (c *Client) UpdateConnection(connectionID string, update map[string]interface{}) ([]byte, error) {
	endpoint := fmt.Sprintf("/v1/connections/%s", connectionID)
	return c.Patch(endpoint, update)
}

// getServerURL returns the server URL for internal API endpoints
// The state endpoint is on the Airbyte server (web UI), not the public API
func (c *Client) getServerURL() (string, error) {
	serverURL := c.baseURL

	// If using the default public API URL, convert to the cloud server URL
	if strings.HasPrefix(c.baseURL, "https://api.airbyte.com") {
		serverURL = "https://cloud.airbyte.com"
	}

	// Parse the server URL to get the root
	parsedURL, err := url.Parse(serverURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse server URL: %w", err)
	}

	return parsedURL.Scheme + "://" + parsedURL.Host, nil
}

// GetConnectionState retrieves the state for a specific connection
// Uses the internal /api/v1/state/get endpoint which is only available on the Airbyte server URL (not the public API)
func (c *Client) GetConnectionState(connectionID string) ([]byte, error) {
	// Get a valid access token first
	if err := c.getAccessToken(); err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	serverRoot, err := c.getServerURL()
	if err != nil {
		return nil, err
	}

	stateURL, err := url.JoinPath(serverRoot, "/api/v1/state/get")
	if err != nil {
		return nil, fmt.Errorf("failed to construct state endpoint URL: %w", err)
	}

	// Prepare the request body
	requestBody := map[string]interface{}{
		"connectionId": connectionID,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Make the POST request
	req, err := http.NewRequest("POST", stateURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("state API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// SetConnectionState sets the state for a specific connection
// Uses the internal /api/v1/state/create_or_update_safe endpoint on the Airbyte server URL
func (c *Client) SetConnectionState(connectionID string, stateData map[string]interface{}) ([]byte, error) {
	// Get a valid access token first
	if err := c.getAccessToken(); err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	serverRoot, err := c.getServerURL()
	if err != nil {
		return nil, err
	}

	stateURL, err := url.JoinPath(serverRoot, "/api/v1/state/create_or_update_safe")
	if err != nil {
		return nil, fmt.Errorf("failed to construct state endpoint URL: %w", err)
	}

	// connectionId is expected to already be set in stateData by the caller
	// (redundant set removed to avoid mutating caller's map)

	jsonData, err := json.Marshal(stateData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state data: %w", err)
	}

	req, err := http.NewRequest("POST", stateURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("set state API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return respBody, nil
}
