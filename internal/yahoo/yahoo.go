package yahoo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"
)

type Quote struct {
	Symbol          string
	Price           float64
	Change          float64
	ChangePercent   float64
	MarketState     string
	FiftyTwoWeekHigh float64
	PctFromHigh     float64
}

type chartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Symbol             string  `json:"symbol"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				ChartPreviousClose float64 `json:"chartPreviousClose"`
				FiftyTwoWeekHigh   float64 `json:"fiftyTwoWeekHigh"`
			} `json:"meta"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

type Client struct {
	httpClient *http.Client
	cookieJar  *cookiejar.Jar
	crumb      string
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

	quotes := make(map[string]Quote)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, symbol := range symbols {
		wg.Add(1)
		go func(sym string) {
			defer wg.Done()
			quote, err := c.fetchQuote(sym)
			if err == nil && quote != nil {
				mu.Lock()
				quotes[sym] = *quote
				mu.Unlock()
			}
		}(symbol)
	}

	wg.Wait()
	return quotes, nil
}

func (c *Client) fetchQuote(symbol string) (*Quote, error) {
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=1d", symbol)

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

	var cr chartResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, err
	}

	if cr.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo API error: %s", cr.Chart.Error.Description)
	}

	if len(cr.Chart.Result) == 0 {
		return nil, fmt.Errorf("no data for symbol %s", symbol)
	}

	meta := cr.Chart.Result[0].Meta
	change := meta.RegularMarketPrice - meta.ChartPreviousClose
	changePercent := 0.0
	if meta.ChartPreviousClose > 0 {
		changePercent = (change / meta.ChartPreviousClose) * 100
	}

	pctFromHigh := 0.0
	if meta.FiftyTwoWeekHigh > 0 {
		pctFromHigh = ((meta.RegularMarketPrice - meta.FiftyTwoWeekHigh) / meta.FiftyTwoWeekHigh) * 100
	}

	return &Quote{
		Symbol:           meta.Symbol,
		Price:            meta.RegularMarketPrice,
		Change:           change,
		ChangePercent:    changePercent,
		FiftyTwoWeekHigh: meta.FiftyTwoWeekHigh,
		PctFromHigh:      pctFromHigh,
	}, nil
}

func (c *Client) GetQuote(symbol string) (*Quote, error) {
	return c.fetchQuote(symbol)
}
