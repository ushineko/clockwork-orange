package plugins

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// httpGet performs one GET with a per-request deadline and optional headers,
// returning the status code and the whole body — the shape requests.get(...)
// followed by .status_code/.content gave the Python plugins. The body is
// always closed before returning.
func httpGet(ctx context.Context, client *http.Client, rawURL string, params url.Values,
	timeout time.Duration, headers map[string]string,
) (int, []byte, error) {
	if len(params) > 0 {
		u, err := url.Parse(rawURL)
		if err != nil {
			return 0, nil, fmt.Errorf("parse url %q: %w", rawURL, err)
		}
		q := u.Query()
		for k, vs := range params {
			for _, v := range vs {
				q.Add(k, v)
			}
		}
		u.RawQuery = q.Encode()
		rawURL = u.String()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("get %s: %w", req.URL.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read response body: %w", err)
	}
	return resp.StatusCode, body, nil
}
