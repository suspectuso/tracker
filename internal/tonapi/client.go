package tonapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tonkeeper/tongo/ton"
)

// Client is a TonAPI HTTP client
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client

	// Rate limiting
	mu         sync.Mutex
	lastCall   time.Time
	minDelay   time.Duration
}

// NewClient creates a new TonAPI client
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		minDelay: 250 * time.Millisecond, // ~4 RPS
	}
}

func (c *Client) throttle() {
	c.mu.Lock()
	defer c.mu.Unlock()

	elapsed := time.Since(c.lastCall)
	if elapsed < c.minDelay {
		time.Sleep(c.minDelay - elapsed)
	}
	c.lastCall = time.Now()
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	c.throttle()

	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	return data, nil
}

// GetAccountInfo returns account information
func (c *Client) GetAccountInfo(ctx context.Context, address string) (*AccountInfo, error) {
	data, err := c.doRequest(ctx, "GET", "/accounts/"+address, nil)
	if err != nil {
		return nil, err
	}

	var info AccountInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	return &info, nil
}

// GetEvents returns recent events for an account
func (c *Client) GetEvents(ctx context.Context, address string, limit int) ([]Event, error) {
	path := fmt.Sprintf("/accounts/%s/events?limit=%d", address, limit)
	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp EventsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	return resp.Events, nil
}

// GetEventByHash returns an event by transaction hash
func (c *Client) GetEventByHash(ctx context.Context, txHash string) (*Event, error) {
	data, err := c.doRequest(ctx, "GET", "/events/"+txHash, nil)
	if err != nil {
		return nil, err
	}

	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	return &event, nil
}

// --- Webhook Management ---

// ListWebhooks returns all webhooks
func (c *Client) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	data, err := c.doRequest(ctx, "GET", "/webhooks", nil)
	if err != nil {
		return nil, err
	}

	var resp WebhookListResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	return resp.Webhooks, nil
}

// CreateWebhook creates a new webhook
func (c *Client) CreateWebhook(ctx context.Context, endpoint string) (*Webhook, error) {
	body := map[string]string{"endpoint": endpoint}
	data, err := c.doRequest(ctx, "POST", "/webhooks", body)
	if err != nil {
		return nil, err
	}

	var webhook Webhook
	if err := json.Unmarshal(data, &webhook); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	return &webhook, nil
}

// DeleteWebhook deletes a webhook
func (c *Client) DeleteWebhook(ctx context.Context, webhookID int64) error {
	path := fmt.Sprintf("/webhooks/%d", webhookID)
	_, err := c.doRequest(ctx, "DELETE", path, nil)
	return err
}

// SubscribeAccounts subscribes accounts to a webhook
func (c *Client) SubscribeAccounts(ctx context.Context, webhookID int64, accounts []string) error {
	path := fmt.Sprintf("/webhooks/%d/account-tx/subscribe", webhookID)
	body := map[string][]string{"accounts": accounts}
	_, err := c.doRequest(ctx, "POST", path, body)
	return err
}

// UnsubscribeAccounts unsubscribes accounts from a webhook
func (c *Client) UnsubscribeAccounts(ctx context.Context, webhookID int64, accounts []string) error {
	path := fmt.Sprintf("/webhooks/%d/account-tx/unsubscribe", webhookID)
	body := map[string][]string{"accounts": accounts}
	_, err := c.doRequest(ctx, "POST", path, body)
	return err
}

// --- Address Utilities ---

// NanoToTON converts nanoTON to TON
func NanoToTON(nano int64) float64 {
	return float64(nano) / 1e9
}

// JettonUnitsToAmount converts jetton units to human-readable amount
func JettonUnitsToAmount(units string, decimals int) float64 {
	val, err := strconv.ParseInt(units, 10, 64)
	if err != nil {
		return 0
	}
	divisor := 1.0
	for i := 0; i < decimals; i++ {
		divisor *= 10
	}
	return float64(val) / divisor
}

// RawToFriendly converts raw address (0:...) to friendly format (UQ.../EQ...)
func RawToFriendly(raw string) string {
	if raw == "" {
		return ""
	}

	// Try to parse using tongo
	acc, err := ton.ParseAccountID(raw)
	if err != nil {
		return raw
	}

	// Convert to user-friendly format (bounceable, URL-safe)
	return acc.ToHuman(true, false)
}

// NormalizeAddress converts any address format to raw (0:...)
func NormalizeAddress(addr string) string {
	if addr == "" {
		return ""
	}

	// Try to parse using tongo
	acc, err := ton.ParseAccountID(addr)
	if err != nil {
		return addr
	}

	return acc.String()
}

// ShortAddr returns a shortened address for display
func ShortAddr(addr string, n int) string {
	if addr == "" {
		return "unknown"
	}
	if len(addr) < n*2+3 {
		return addr
	}
	return addr[:n] + "..." + addr[len(addr)-n:]
}
