package epay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// OrderInfo represents order information from query
type OrderInfo struct {
	Code        int    `json:"code"`
	Msg         string `json:"msg"`
	TradeNo     string `json:"trade_no"`      // Epay order number
	OutTradeNo  string `json:"out_trade_no"`  // Merchant order number
	APITradeNo  string `json:"api_trade_no"`  // Third-party order number
	Type        string `json:"type"`          // Payment type
	PID         int    `json:"pid"`           // Merchant ID
	AddTime     string `json:"addtime"`       // Order creation time
	EndTime     string `json:"endtime"`       // Order completion time
	Name        string `json:"name"`          // Product name
	Money       string `json:"money"`         // Amount
	Status      int    `json:"status"`        // 1 for paid, 0 for unpaid
	Param       string `json:"param"`         // Business parameter
	Buyer       string `json:"buyer"`         // Payer account
}

// QueryOrder queries a single order by trade_no or out_trade_no
func (c *Client) QueryOrder(tradeNo, outTradeNo string) (*OrderInfo, error) {
	params := url.Values{}
	params.Set("act", "order")
	params.Set("pid", c.PID)
	params.Set("key", c.Key)
	
	if tradeNo != "" {
		params.Set("trade_no", tradeNo)
	} else if outTradeNo != "" {
		params.Set("out_trade_no", outTradeNo)
	} else {
		return nil, fmt.Errorf("either trade_no or out_trade_no must be provided")
	}
	
	resp, err := http.Get(c.Gateway + "/api.php?" + params.Encode())
	if err != nil {
		return nil, fmt.Errorf("failed to query order: %w", err)
	}
	defer resp.Body.Close()
	
	var result OrderInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	if result.Code != 1 {
		return nil, fmt.Errorf("query failed: %s", result.Msg)
	}
	
	return &result, nil
}

// RefundRequest represents a refund request
type RefundRequest struct {
	TradeNo    string  // Epay order number
	OutTradeNo string  // Merchant order number
	Money      float64 // Refund amount
}

// RefundResponse represents refund response
type RefundResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// RefundOrder submits a refund request
func (c *Client) RefundOrder(req RefundRequest) error {
	values := url.Values{}
	values.Set("pid", c.PID)
	values.Set("key", c.Key)
	
	if req.TradeNo != "" {
		values.Set("trade_no", req.TradeNo)
	} else if req.OutTradeNo != "" {
		values.Set("out_trade_no", req.OutTradeNo)
	} else {
		return fmt.Errorf("either trade_no or out_trade_no must be provided")
	}
	
	values.Set("money", fmt.Sprintf("%.2f", req.Money))
	
	resp, err := http.PostForm(c.Gateway+"/api.php?act=refund", values)
	if err != nil {
		return fmt.Errorf("failed to submit refund: %w", err)
	}
	defer resp.Body.Close()
	
	var result RefundResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}
	
	if result.Code != 0 { // Note: refund API returns 0 for success
		return fmt.Errorf("refund failed: %s", result.Msg)
	}
	
	return nil
}

// MerchantInfo represents merchant information
type MerchantInfo struct {
	Code         int    `json:"code"`
	PID          int    `json:"pid"`
	Key          string `json:"key"`
	Active       int    `json:"active"`        // 1 for active, 0 for banned
	Money        string `json:"money"`         // Balance
	Type         int    `json:"type"`          // Settlement type
	Account      string `json:"account"`       // Settlement account
	Username     string `json:"username"`      // Account name
	Orders       int    `json:"orders"`        // Total orders
	OrderToday   int    `json:"order_today"`   // Today's orders
	OrderLastday int    `json:"order_lastday"` // Yesterday's orders
}

// QueryMerchantInfo queries merchant account information
func (c *Client) QueryMerchantInfo() (*MerchantInfo, error) {
	params := url.Values{}
	params.Set("act", "query")
	params.Set("pid", c.PID)
	params.Set("key", c.Key)
	
	resp, err := http.Get(c.Gateway + "/api.php?" + params.Encode())
	if err != nil {
		return nil, fmt.Errorf("failed to query merchant info: %w", err)
	}
	defer resp.Body.Close()
	
	var result MerchantInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	if result.Code != 1 {
		return nil, fmt.Errorf("query failed")
	}
	
	return &result, nil
}