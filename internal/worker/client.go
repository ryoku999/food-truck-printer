package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{baseURL, token, &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (c *Client) Post(ctx context.Context, path string, body, output any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("API %s: HTTP %d", path, res.StatusCode)
	}
	var response struct {
		Success bool
		Data    json.RawMessage
	}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return err
	}
	if !response.Success || len(response.Data) == 0 || bytes.Equal(response.Data, []byte("null")) {
		return fmt.Errorf("API %s: respuesta sin datos", path)
	}
	return json.Unmarshal(response.Data, output)
}
