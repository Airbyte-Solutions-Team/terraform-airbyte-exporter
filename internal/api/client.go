package api

import (
	"bytes"
	"encoding/base64"
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
	username     string
	password     string
	httpClient   *http.Client
	accessToken  string
	tokenExpiry  time.Time
}

// NewClient creates a new API client
func NewClient(baseURL, clientID, clientSecret, username, password string) (*Client, error) {
	// Validate that only one authentication method is provided
	hasOAuth2 := clientID != "" || clientSecret != ""
	hasBasicAuth := username != "" || password != ""

	if hasOAuth2 && hasBasicAuth {
		return nil, fmt.Errorf("cannot use both OAuth2 (client_id/client_secret) and basic auth (username/password) simultaneously - please choose one authentication method")
	}

	if !hasOAuth2 && !hasBasicAuth {
		return nil, fmt.Errorf("authentication required: provide either OAuth2 credentials (client_id/client_secret) or basic auth credentials (username/password)")
	}

	// Validate OAuth2 credentials are complete
	if hasOAuth2 && (clientID == "" || clientSecret == "") {
		return nil, fmt.Errorf("incomplete OAuth2 credentials: both client_id and client_secret are required")
	}

	// Validate basic auth credentials are complete
	if hasBasicAuth && (username == "" || password == "") {
		return nil, fmt.Errorf("incomplete basic auth credentials: both username and password are required")
	}

	return &Client{
		baseURL:      baseURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		username:     username,
		password:     password,
		httpClient:   &http.Client{},
	}, nil
}

// encodeBasicAuth encodes username and password for basic authentication
func (c *Client) encodeBasicAuth() string {
	auth := c.username + ":" + c.password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

// useBasicAuth returns true if username and password are configured
func (c *Client) useBasicAuth() bool {
	return c.username != "" && c.password != ""
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
	tokenURL, err := url.JoinPath(c.baseURL, "v1", "applications", "token")
	if err != nil {
		return fmt.Errorf("failed to construct token URL: %w", err)
	}
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

// Get makes a GET request to the specified endpoint
func (c *Client) Get(endpoint string, workspaceID *string) ([]byte, error) {
	fullURL, err := url.JoinPath(c.baseURL, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to construct URL: %w", err)
	}

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set authentication header
	if c.useBasicAuth() {
		// Use basic authentication
		req.Header.Set("Authorization", "Basic "+c.encodeBasicAuth())
	} else {
		// Get a valid access token first
		if err := c.getAccessToken(); err != nil {
			return nil, fmt.Errorf("failed to get access token: %w", err)
		}
		// Use bearer token authentication
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}

	// Add workspace ID as query parameter if provided
	if workspaceID != nil && *workspaceID != "" {
		q := req.URL.Query()
		q.Add("workspaceId", *workspaceID)
		req.URL.RawQuery = q.Encode()
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// Post makes a POST request to the specified endpoint
func (c *Client) Post(endpoint string, data interface{}) ([]byte, error) {
	fullURL, err := url.JoinPath(c.baseURL, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to construct URL: %w", err)
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	req, err := http.NewRequest("POST", fullURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Set authentication header
	if c.useBasicAuth() {
		// Use basic authentication
		req.Header.Set("Authorization", "Basic "+c.encodeBasicAuth())
	} else {
		// Get a valid access token first
		if err := c.getAccessToken(); err != nil {
			return nil, fmt.Errorf("failed to get access token: %w", err)
		}
		// Use bearer token authentication
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}

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

// internalBaseURL returns the base URL for the internal config API
// by stripping "/public" from the public API base URL.
// e.g., "https://host/api/public" -> "https://host/api"
func (c *Client) internalBaseURL() string {
	return strings.TrimSuffix(c.baseURL, "/public")
}

// PostInternal makes a POST request to the internal config API
func (c *Client) PostInternal(endpoint string, data interface{}) ([]byte, error) {
	fullURL, err := url.JoinPath(c.internalBaseURL(), endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to construct URL: %w", err)
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	req, err := http.NewRequest("POST", fullURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Set authentication header
	if c.useBasicAuth() {
		req.Header.Set("Authorization", "Basic "+c.encodeBasicAuth())
	} else {
		if err := c.getAccessToken(); err != nil {
			return nil, fmt.Errorf("failed to get access token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}

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

// GetCustomSourceDefinitions fetches custom source definitions for a workspace via the internal config API
func (c *Client) GetCustomSourceDefinitions(workspaceID string) ([]byte, error) {
	return c.PostInternal("/v1/source_definitions/list_for_workspace", map[string]string{"workspaceId": workspaceID})
}

// GetCustomDestinationDefinitions fetches custom destination definitions for a workspace via the internal config API
func (c *Client) GetCustomDestinationDefinitions(workspaceID string) ([]byte, error) {
	return c.PostInternal("/v1/destination_definitions/list_for_workspace", map[string]string{"workspaceId": workspaceID})
}
