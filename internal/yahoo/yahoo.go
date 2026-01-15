package yahoo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Quote struct {
	Symbol        string
	Price         float64
	Change        float64
	ChangePercent float64
	MarketState   string
}

type quoteResponse struct {
	QuoteResponse struct {
		Result []struct {
			Symbol             string  `json:"symbol"`
			RegularMarketPrice float64 `json:"regularMarketPrice"`
			RegularMarketChange float64 `json:"regularMarketChange"`
			RegularMarketChangePercent float64 `json:"regularMarketChangePercent"`
			MarketState        string  `json:"marketState"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"quoteResponse"`
}

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) GetQuotes(symbols []string) (map[string]Quote, error) {
	if len(symbols) == 0 {
		return make(map[string]Quote), nil
	}

	// Build comma-separated symbol list
	symbolList := ""
	for i, s := range symbols {
		if i > 0 {
			symbolList += ","
		}
		symbolList += s
	}

	url := fmt.Sprintf("https://query1.finance.yahoo.com/v7/finance/quote?symbols=%s", symbolList)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo API returned status %d", resp.StatusCode)
	}

	var qr quoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, err
	}

	if qr.QuoteResponse.Error != nil {
		return nil, fmt.Errorf("yahoo API error: %s", qr.QuoteResponse.Error.Description)
	}

	quotes := make(map[string]Quote)
	for _, r := range qr.QuoteResponse.Result {
		quotes[r.Symbol] = Quote{
			Symbol:        r.Symbol,
			Price:         r.RegularMarketPrice,
			Change:        r.RegularMarketChange,
			ChangePercent: r.RegularMarketChangePercent,
			MarketState:   r.MarketState,
		}
	}

	return quotes, nil
}

func (c *Client) GetQuote(symbol string) (*Quote, error) {
	quotes, err := c.GetQuotes([]string{symbol})
	if err != nil {
		return nil, err
	}
	if q, ok := quotes[symbol]; ok {
		return &q, nil
	}
	return nil, fmt.Errorf("no quote found for %s", symbol)
}
