package ynab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Transaction represents a YNAB transaction to be created
type Transaction struct {
	AccountID string `json:"account_id"`
	Date      string `json:"date"`
	Amount    int64  `json:"amount"`
	PayeeName string `json:"payee_name"`
	Memo      string `json:"memo,omitempty"`
	Cleared   string `json:"cleared,omitempty"`
	Approved  bool   `json:"approved"`
}

type CreateTransactionRequest struct {
	Transaction Transaction `json:"transaction"`
}

// CreateTransaction posts a single transaction to YNAB
func (c *Client) CreateTransaction(tx Transaction) error {
	url := fmt.Sprintf("%s/budgets/%s/transactions", baseURL, c.budgetID)
	reqBody := CreateTransactionRequest{Transaction: tx}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal transaction: %w", err)
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated {
		var errResp ErrorResponse
		_ = json.Unmarshal(respBody, &errResp)
		return fmt.Errorf("YNAB API error %d: %s - %s", resp.StatusCode, errResp.Error.Name, errResp.Error.Detail)
	}
	return nil
}

const baseURL = "https://api.ynab.com/v1"

type Client struct {
	accessToken string
	budgetID    string
	httpClient  *http.Client
}

type Account struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Balance int64  `json:"balance"`
	Note    string `json:"note,omitempty"`
	Closed  bool   `json:"closed"`
}

type AccountsResponse struct {
	Data struct {
		Accounts []Account `json:"accounts"`
	} `json:"data"`
}

type UpdateAccountRequest struct {
	Account struct {
		Cleared   int64 `json:"cleared"`
		Uncleared int64 `json:"uncleared,omitempty"`
	} `json:"account"`
}

type ErrorResponse struct {
	Error struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Detail string `json:"detail"`
	} `json:"error"`
}

func NewClient(accessToken, budgetID string) *Client {
	return &Client{
		accessToken: accessToken,
		budgetID:    budgetID,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// GetAccounts retrieves all accounts in the specified budget
func (c *Client) GetAccounts() ([]Account, error) {
	url := fmt.Sprintf("%s/budgets/%s/accounts", baseURL, c.budgetID)

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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		json.Unmarshal(body, &errResp)
		return nil, fmt.Errorf("API returned status %d: %s - %s", resp.StatusCode, errResp.Error.Name, errResp.Error.Detail)
	}

	var accountsResp AccountsResponse
	if err := json.Unmarshal(body, &accountsResp); err != nil {
		return nil, fmt.Errorf("failed to parse accounts response: %w", err)
	}

	return accountsResp.Data.Accounts, nil
}

// UpdateAccountBalance updates the cleared balance for an account
// amount should be in milliunits (multiply by 1000 if in regular units)
func (c *Client) UpdateAccountBalance(accountID string, amountMilliunits int64) error {
	url := fmt.Sprintf("%s/budgets/%s/accounts/%s", baseURL, c.budgetID, accountID)

	updateReq := UpdateAccountRequest{}
	updateReq.Account.Cleared = amountMilliunits

	body, err := json.Marshal(updateReq)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		json.Unmarshal(respBody, &errResp)
		return fmt.Errorf("API returned status %d: %s - %s", resp.StatusCode, errResp.Error.Name, errResp.Error.Detail)
	}

	return nil
}

// GetBudgets retrieves all available budgets
func (c *Client) GetBudgets() ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/budgets", baseURL)

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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		json.Unmarshal(body, &errResp)
		return nil, fmt.Errorf("API returned status %d: %s - %s", resp.StatusCode, errResp.Error.Name, errResp.Error.Detail)
	}

	var budgetsResp map[string]interface{}
	if err := json.Unmarshal(body, &budgetsResp); err != nil {
		return nil, fmt.Errorf("failed to parse budgets response: %w", err)
	}

	data := budgetsResp["data"].(map[string]interface{})
	budgets := data["budgets"].([]interface{})
	var result []map[string]interface{}
	for _, b := range budgets {
		result = append(result, b.(map[string]interface{}))
	}

	return result, nil
}
