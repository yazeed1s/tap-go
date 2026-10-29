package tap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// do sends a JSON request and decodes a JSON response into out.
func (c *Client) do(
	ctx context.Context,
	method, path string,
	in, out any,
) error {
	body, err := c.doRaw(ctx, method, path, in)
	if err != nil {
		return err
	}
	return decodeJSON(body, out)
}

func decodeJSON(body []byte, out any) error {
	if out == nil || len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("tap: decode response: %w", err)
	}
	return nil
}

// doRaw sends a JSON request and returns the raw response body. It is used
// directly by endpoints that return CSV, such as the download operations.
func (c *Client) doRaw(
	ctx context.Context,
	method, path string,
	in any,
) ([]byte, error) {
	var body io.Reader
	if in != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(in); err != nil {
			return nil, fmt.Errorf("tap: encode request: %w", err)
		}
		body = &buf
	}
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.send(req)
}

func (c *Client) newRequest(
	ctx context.Context,
	method, path string,
	body io.Reader,
) (*http.Request, error) {
	if c.baseURL == nil {
		return nil, fmt.Errorf("tap: base URL is not configured")
	}
	// path arrives with its parameters already escaped, so it is parsed
	// rather than assigned to URL.Path, which would escape it again.
	u, err := url.Parse(
		strings.TrimRight(
			c.baseURL.String(),
			"/",
		) + "/" + strings.TrimLeft(
			path,
			"/",
		),
	)
	if err != nil {
		return nil, fmt.Errorf("tap: build request URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if c.language != "" {
		req.Header.Set("lang_code", c.language)
	}
	return req, nil
}

func (c *Client) send(req *http.Request) ([]byte, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tap: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tap: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, decodeAPIError(resp, body)
	}
	return body, nil
}
