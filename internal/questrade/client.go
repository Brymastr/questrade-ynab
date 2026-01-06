package questrade

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const productionAuthURL = "https://login.questrade.com/oauth2/token"

type Client struct {
	refreshToken string
	accessToken  string
	apiServer    string
	httpClient   *http.Client
	expiresAt    time.Time
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	APIServer    string `json:"api_server"`
}

type Account struct {
	Type              string           `json:"type"`
	Number            string           `json:"number"`
	Status            string           `json:"status"`
	IsPrimary         bool             `json:"isPrimary"`
	IsBilling         bool             `json:"isBilling"`
	ClientAccountType string           `json:"clientAccountType"`
	Balances          *AccountBalances `json:"-"` // Populated by GetAccounts in parallel
}

type Balance struct {
	USD    float64 `json:"usd"`
	CAD    float64 `json:"cad"`
	Total  float64 `json:"total"`
	Market float64 `json:"market"`
}

type PerCurrencyBalance struct {
	Currency          string  `json:"currency"`
	Cash              float64 `json:"cash"`
	MarketValue       float64 `json:"marketValue"`
	TotalEquity       float64 `json:"totalEquity"`
	BuyingPower       float64 `json:"buyingPower"`
	MaintenanceExcess float64 `json:"maintenanceExcess"`
	IsRealTime        bool    `json:"isRealTime"`
}

type AccountBalances struct {
	PerCurrencyBalances []PerCurrencyBalance `json:"perCurrencyBalances"`
	CombinedBalances    []PerCurrencyBalance `json:"combinedBalances"`
}

type AccountsResponse struct {
	Accounts []Account `json:"accounts"`
	UserID   int       `json:"userId"`
}

func NewClient(refreshToken string) *Client {
	return &Client{
		refreshToken: refreshToken,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Refresh exchanges the stored refresh token for a short-lived access token and API server
// It returns the parsed token response so callers may persist values as needed.
func (c *Client) Refresh() (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", c.refreshToken)

	// Build full URL (for logging) and log the request we're about to send so we can debug endpoint issues
	fullURL := productionAuthURL + "?" + data.Encode()
	log.Printf("questrade token refresh request: POST %s body=%s", fullURL, data.Encode())

	resp, err := c.httpClient.PostForm(productionAuthURL, data)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Always log the response body and status so we can inspect successful replies too
	log.Printf("questrade token refresh response: status=%d body=%s", resp.StatusCode, string(body))

	// Log the raw response body and status for debugging endpoint issues
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed: status %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		log.Printf("failed to parse token response body: %s", string(body))
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		log.Printf("token response missing access_token: %s", string(body))
		return nil, fmt.Errorf("no access token in response")
	}

	c.accessToken = tokenResp.AccessToken
	c.apiServer = tokenResp.APIServer
	c.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	// update refresh token if provided
	if tokenResp.RefreshToken != "" {
		c.refreshToken = tokenResp.RefreshToken
	}

	return &tokenResp, nil
}

// GetRefreshToken returns the current refresh token (may have been rotated)
func (c *Client) GetRefreshToken() string {
	return c.refreshToken
}

// GetAPIServer returns the api server URL discovered from the token response
func (c *Client) GetAPIServer() string {
	return c.apiServer
}

// GetAccessToken returns the current access token
func (c *Client) GetAccessToken() string {
	return c.accessToken
}

// GetExpiresAt returns the expiration time of the current access token
func (c *Client) GetExpiresAt() time.Time {
	return c.expiresAt
}

// SetAccessToken sets the access token, api server and expiration directly (used when loading cached token)
func (c *Client) SetAccessToken(accessToken, apiServer string, expiresIn int) {
	c.accessToken = accessToken
	c.apiServer = apiServer
	c.expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
}

// IsTokenValid returns true if the current access token is still valid
func (c *Client) IsTokenValid() bool {
	if c.accessToken == "" {
		return false
	}
	return time.Now().Before(c.expiresAt)
}

// IsAccessTokenValid performs a live check against the Questrade API using the provided
// access token and apiServer. It calls the /v1/time endpoint. If the endpoint returns
// 200 the token is valid. If it returns 401 or a body indicating an invalid token the
// token is considered invalid. Any other non-200 status returns an error.
func (c *Client) IsAccessTokenValid(accessToken, apiServer string) (bool, error) {
	if accessToken == "" || apiServer == "" {
		return false, nil
	}

	// Ensure apiServer does not have a trailing slash duplication
	urlStr := fmt.Sprintf("%s/v1/time", strings.TrimRight(apiServer, "/"))
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	// 401 indicates invalid token
	if resp.StatusCode == http.StatusUnauthorized {
		return false, nil
	}

	body, _ := io.ReadAll(resp.Body)
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "invalid") && strings.Contains(lower, "access") {
		return false, nil
	}

	return false, fmt.Errorf("unexpected status from token validate: %d - %s", resp.StatusCode, string(body))
}

// GetAccountBalances retrieves balance information for an account
func (c *Client) GetAccountBalances(accountNumber string) (*Balance, error) {
	if c.accessToken == "" {
		return nil, fmt.Errorf("not authenticated, call Refresh first")
	}

	url := fmt.Sprintf("%s/v1/accounts/%s/balances", c.apiServer, accountNumber)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var balanceResp struct {
		CombinedBalances []Balance `json:"combinedBalances"`
	}
	if err := json.Unmarshal(body, &balanceResp); err != nil {
		return nil, fmt.Errorf("failed to parse balance response: %w", err)
	}

	if len(balanceResp.CombinedBalances) > 0 {
		return &balanceResp.CombinedBalances[0], nil
	}

	return nil, fmt.Errorf("no balance data found")
}

// GetAccountBalancesByID retrieves detailed balance information for an account by account ID.
// Returns per-currency and combined balances from the /v1/accounts/{id}/balances endpoint.
func (c *Client) GetAccountBalancesByID(accountID string) (*AccountBalances, error) {
	if c.accessToken == "" {
		return nil, fmt.Errorf("not authenticated, call Refresh first")
	}

	url := fmt.Sprintf("%sv1/accounts/%s/balances", c.apiServer, accountID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var balances AccountBalances
	if err := json.Unmarshal(body, &balances); err != nil {
		return nil, fmt.Errorf("failed to parse balance response: %w", err)
	}

	return &balances, nil
}

// GetAccounts retrieves all accounts
func (c *Client) GetAccounts() ([]Account, error) {
	if c.accessToken == "" {
		return nil, fmt.Errorf("not authenticated, call Refresh first")
	}

	url := fmt.Sprintf("%sv1/accounts", c.apiServer)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var accountsResp AccountsResponse
	if err := json.Unmarshal(body, &accountsResp); err != nil {
		return nil, fmt.Errorf("failed to parse accounts response: %w", err)
	}

	// Fetch balances for each account in parallel
	var wg sync.WaitGroup
	wg.Add(len(accountsResp.Accounts))
	for i := range accountsResp.Accounts {
		go func(idx int) {
			defer wg.Done()
			balances, err := c.GetAccountBalancesByID(accountsResp.Accounts[idx].Number)
			if err != nil {
				log.Printf("Warning: failed to fetch balances for account %s: %v", accountsResp.Accounts[idx].Number, err)
				return
			}
			accountsResp.Accounts[idx].Balances = balances
		}(i)
	}
	wg.Wait()

	return accountsResp.Accounts, nil
}
