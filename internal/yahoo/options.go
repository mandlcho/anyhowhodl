package yahoo

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"

	"anyhowhodl/internal/csp"
)

// crumb + cookie fields are stored on the Client.

// optionsResponse maps the /v7/finance/options/ JSON response.
type optionsResponse struct {
	OptionChain struct {
		Result []struct {
			UnderlyingSymbol string  `json:"underlyingSymbol"`
			ExpirationDates  []int64 `json:"expirationDates"`
			Quote            struct {
				RegularMarketPrice float64 `json:"regularMarketPrice"`
			} `json:"quote"`
			Options []struct {
				ExpirationDate int64           `json:"expirationDate"`
				Calls          []optionRawItem `json:"calls"`
				Puts           []optionRawItem `json:"puts"`
			} `json:"options"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"optionChain"`
}

type optionRawItem struct {
	ContractSymbol    string  `json:"contractSymbol"`
	Strike            float64 `json:"strike"`
	Currency          string  `json:"currency"`
	LastPrice         float64 `json:"lastPrice"`
	Change            float64 `json:"change"`
	PercentChange     float64 `json:"percentChange"`
	Volume            int     `json:"volume"`
	OpenInterest      int     `json:"openInterest"`
	Bid               float64 `json:"bid"`
	Ask               float64 `json:"ask"`
	Expiration        int64   `json:"expiration"`
	ImpliedVolatility float64 `json:"impliedVolatility"`
	InTheMoney        bool    `json:"inTheMoney"`
}

// chartHistoryResponse maps the /v8/finance/chart/ JSON response for range=1y.
type chartHistoryResponse struct {
	Chart struct {
		Result []struct {
			Indicators struct {
				Quote []struct {
					Close []*float64 `json:"close"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

// ensureCrumb fetches crumb and cookies if not already present.
func (c *Client) ensureCrumb() error {
	if c.crumb != "" {
		return nil
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("creating cookie jar: %w", err)
	}

	// Step 1: GET fc.yahoo.com to get cookies
	client := &http.Client{
		Timeout: 10 * time.Second,
		Jar:     jar,
	}

	req, err := http.NewRequest("GET", "https://fc.yahoo.com", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching cookies: %w", err)
	}
	resp.Body.Close()

	// Step 2: GET crumb using those cookies
	req, err = http.NewRequest("GET", "https://query2.finance.yahoo.com/v1/test/getcrumb", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching crumb: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("crumb endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading crumb: %w", err)
	}

	c.crumb = string(body)
	c.cookieJar = jar
	return nil
}

// FetchOptionsChain fetches the options chain for the default (nearest) expiry.
func (c *Client) FetchOptionsChain(ticker string) (*csp.OptionsData, error) {
	return c.fetchOptions(ticker, 0)
}

// FetchOptionsChainForExpiry fetches the options chain for a specific expiry timestamp.
func (c *Client) FetchOptionsChainForExpiry(ticker string, expiry int64) (*csp.OptionsData, error) {
	return c.fetchOptions(ticker, expiry)
}

func (c *Client) fetchOptions(ticker string, expiry int64) (*csp.OptionsData, error) {
	if err := c.ensureCrumb(); err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	time.Sleep(200 * time.Millisecond)

	url := fmt.Sprintf("https://query1.finance.yahoo.com/v7/finance/options/%s?crumb=%s", ticker, c.crumb)
	if expiry > 0 {
		url = fmt.Sprintf("%s&date=%d", url, expiry)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	client := &http.Client{
		Timeout: 10 * time.Second,
		Jar:     c.cookieJar,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo options API returned status %d", resp.StatusCode)
	}

	var or optionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return nil, err
	}

	return parseOptionsResponse(&or)
}

func parseOptionsResponse(or *optionsResponse) (*csp.OptionsData, error) {
	if or.OptionChain.Error != nil {
		return nil, fmt.Errorf("yahoo API error: %s", or.OptionChain.Error.Description)
	}
	if len(or.OptionChain.Result) == 0 {
		return nil, fmt.Errorf("no options data in response")
	}

	r := or.OptionChain.Result[0]
	data := &csp.OptionsData{
		UnderlyingPrice: r.Quote.RegularMarketPrice,
		ExpirationDates: r.ExpirationDates,
	}

	if len(r.Options) > 0 {
		opts := r.Options[0]
		for _, raw := range opts.Puts {
			data.Puts = append(data.Puts, csp.OptionContract{
				Strike:            raw.Strike,
				LastPrice:         raw.LastPrice,
				Bid:               raw.Bid,
				Ask:               raw.Ask,
				Volume:            raw.Volume,
				OpenInterest:      raw.OpenInterest,
				ImpliedVolatility: raw.ImpliedVolatility,
				Expiration:        raw.Expiration,
			})
		}
		for _, raw := range opts.Calls {
			data.Calls = append(data.Calls, csp.OptionContract{
				Strike:            raw.Strike,
				LastPrice:         raw.LastPrice,
				Bid:               raw.Bid,
				Ask:               raw.Ask,
				Volume:            raw.Volume,
				OpenInterest:      raw.OpenInterest,
				ImpliedVolatility: raw.ImpliedVolatility,
				Expiration:        raw.Expiration,
			})
		}
	}

	return data, nil
}

// FetchPriceHistory fetches 1 year of daily closing prices for a ticker.
func (c *Client) FetchPriceHistory(ticker string) ([]float64, error) {
	time.Sleep(200 * time.Millisecond)

	url := fmt.Sprintf("https://query2.finance.yahoo.com/v8/finance/chart/%s?range=1y&interval=1d", ticker)

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
		return nil, fmt.Errorf("yahoo chart API returned status %d", resp.StatusCode)
	}

	var cr chartHistoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, err
	}

	return parseChartHistoryResponse(&cr)
}

func parseChartHistoryResponse(cr *chartHistoryResponse) ([]float64, error) {
	if cr.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo chart error: %s", cr.Chart.Error.Description)
	}
	if len(cr.Chart.Result) == 0 {
		return nil, fmt.Errorf("no chart data in response")
	}

	quotes := cr.Chart.Result[0].Indicators.Quote
	if len(quotes) == 0 {
		return nil, fmt.Errorf("no quote indicators in chart response")
	}

	rawCloses := quotes[0].Close
	var closes []float64
	for _, v := range rawCloses {
		if v != nil {
			closes = append(closes, *v)
		}
	}

	return closes, nil
}
