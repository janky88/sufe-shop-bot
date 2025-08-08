package epay

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// PaymentType represents the payment method
type PaymentType string

const (
	PaymentAlipay PaymentType = "alipay"
	PaymentWechat PaymentType = "wxpay"
	PaymentQQ     PaymentType = "qqpay"
)

// DeviceType represents the device type
type DeviceType string

const (
	DevicePC     DeviceType = "pc"
	DeviceMobile DeviceType = "mobile"
	DeviceQQ     DeviceType = "qq"
	DeviceWechat DeviceType = "wechat"
	DeviceAlipay DeviceType = "alipay"
	DeviceJump   DeviceType = "jump"
)

// Client is the Epay API client
type Client struct {
	PID     string
	Key     string
	Gateway string
}

// NewClient creates a new Epay client
func NewClient(pid, key, gateway string) *Client {
	return &Client{
		PID:     pid,
		Key:     key,
		Gateway: gateway,
	}
}

// CreateOrderParams contains parameters for creating an order
type CreateOrderParams struct {
	Type       PaymentType // Payment type (optional, defaults to showing all available)
	OutTradeNo string      // Merchant order number
	Name       string      // Product name (max 127 bytes)
	Money      float64     // Amount in yuan
	NotifyURL  string      // Async callback URL
	ReturnURL  string      // Sync return URL
	ClientIP   string      // Client IP address
	Device     DeviceType  // Device type
	Param      string      // Business extension parameter
}

// CreateOrderResponse is the response from CreateOrder
type CreateOrderResponse struct {
	Code      int    `json:"code"` // 1 for success
	Msg       string `json:"msg"`  // Error message
	TradeNo   string `json:"trade_no"`
	PayURL    string `json:"payurl"`    // Direct payment URL
	QRCode    string `json:"qrcode"`    // QR code content
	URLScheme string `json:"urlscheme"` // Mini program URL
}

// CreateOrder creates a payment order using API interface
func (c *Client) CreateOrder(params CreateOrderParams) (*CreateOrderResponse, error) {
	// Validate required parameters
	if params.ClientIP == "" {
		params.ClientIP = "127.0.0.1"
	}
	if params.Device == "" {
		params.Device = DevicePC
	}
	
	// Truncate name if too long
	if len(params.Name) > 127 {
		params.Name = params.Name[:127]
	}
	
	// Build request parameters
	values := url.Values{}
	values.Set("pid", c.PID)
	// Set type if provided, otherwise show all available payment methods
	if params.Type != "" {
		values.Set("type", string(params.Type))
	}
	values.Set("out_trade_no", params.OutTradeNo)
	values.Set("notify_url", params.NotifyURL)
	values.Set("return_url", params.ReturnURL)
	values.Set("name", params.Name)
	values.Set("money", fmt.Sprintf("%.2f", params.Money))
	values.Set("clientip", params.ClientIP)
	values.Set("device", string(params.Device))
	if params.Param != "" {
		values.Set("param", params.Param)
	}
	
	// Generate signature
	sign := c.generateSign(values)
	values.Set("sign", sign)
	values.Set("sign_type", "MD5")
	
	// Send request
	resp, err := http.PostForm(c.Gateway+"/mapi.php", values)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	// Parse JSON response
	var jsonResp CreateOrderResponse
	if err := json.Unmarshal(body, &jsonResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w, body: %s", err, string(body))
	}
	
	if jsonResp.Code != 1 {
		return nil, fmt.Errorf("epay error: %s", jsonResp.Msg)
	}
	
	return &jsonResp, nil
}

// GetPaymentURL returns the appropriate payment URL based on response
func (resp *CreateOrderResponse) GetPaymentURL() string {
	// Priority: payurl > qrcode > urlscheme
	if resp.PayURL != "" {
		return resp.PayURL
	}
	if resp.QRCode != "" {
		// For QR code, we return it as-is for the client to generate QR image
		return resp.QRCode
	}
	if resp.URLScheme != "" {
		return resp.URLScheme
	}
	return ""
}

// IsQRCode checks if the payment method is QR code
func (resp *CreateOrderResponse) IsQRCode() bool {
	return resp.QRCode != "" && resp.PayURL == ""
}

// CreateSubmitURL creates a URL for form submission payment
func (c *Client) CreateSubmitURL(params CreateOrderParams) string {
	// Build parameters
	values := url.Values{}
	values.Set("pid", c.PID)
	// Set type if provided, otherwise show all available payment methods
	if params.Type != "" {
		values.Set("type", string(params.Type))
	}
	values.Set("out_trade_no", params.OutTradeNo)
	values.Set("notify_url", params.NotifyURL)
	values.Set("return_url", params.ReturnURL)
	values.Set("name", params.Name)
	values.Set("money", fmt.Sprintf("%.2f", params.Money))
	if params.Param != "" {
		values.Set("param", params.Param)
	}
	
	// Generate signature
	sign := c.generateSign(values)
	values.Set("sign", sign)
	values.Set("sign_type", "MD5")
	
	return c.Gateway + "/submit.php?" + values.Encode()
}

// VerifyNotify verifies the callback notification
func (c *Client) VerifyNotify(params url.Values) bool {
	// Get the sign from params
	receivedSign := params.Get("sign")
	if receivedSign == "" {
		return false
	}
	
	// Remove sign and sign_type for verification
	paramsCopy := make(url.Values)
	for k, v := range params {
		if k != "sign" && k != "sign_type" {
			paramsCopy[k] = v
		}
	}
	
	// Generate expected sign
	expectedSign := c.generateSign(paramsCopy)
	
	return receivedSign == expectedSign
}

// generateSign generates MD5 signature for parameters
func (c *Client) generateSign(params url.Values) string {
	// Sort parameters by key ASCII order
	var keys []string
	for k := range params {
		// Skip empty values, sign and sign_type
		if k != "" && params.Get(k) != "" && k != "sign" && k != "sign_type" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	
	// Build sign string
	var signParts []string
	for _, k := range keys {
		signParts = append(signParts, fmt.Sprintf("%s=%s", k, params.Get(k)))
	}
	
	// Concatenate with key (no + character)
	signStr := strings.Join(signParts, "&") + c.Key
	
	// Calculate MD5
	h := md5.New()
	h.Write([]byte(signStr))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// ParseNotifyParams extracts common fields from notify parameters
type NotifyParams struct {
	PID         string
	TradeNo     string // Epay order number
	OutTradeNo  string // Merchant order number  
	Type        string // Payment type
	Name        string // Product name
	Money       string // Amount
	TradeStatus string // TRADE_SUCCESS for success
	Param       string // Business extension parameter
}

// ParseNotify parses notification parameters
func ParseNotify(params url.Values) *NotifyParams {
	return &NotifyParams{
		PID:         params.Get("pid"),
		TradeNo:     params.Get("trade_no"),
		OutTradeNo:  params.Get("out_trade_no"),
		Type:        params.Get("type"),
		Name:        params.Get("name"),
		Money:       params.Get("money"),
		TradeStatus: params.Get("trade_status"),
		Param:       params.Get("param"),
	}
}