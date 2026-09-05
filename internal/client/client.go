package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const basePath = "/api"

// Client is a minimal Dokploy API client following the tRPC-OpenAPI
// convention: queries are GETs with query-string inputs, mutations are
// POSTs with JSON bodies.
type Client struct {
	endpoint      string // scheme://host, no trailing slash, without /api
	apiKey        string
	http          *http.Client
	userAgent     string
	retryAttempts int
	retryBaseWait time.Duration
	// requestTimeout bounds one attempt of every call except the deploys.
	// deployTimeout bounds one attempt of the synchronous *.deploy endpoints:
	// on Dokploy v0.30.5 the server pulls, builds, and then waits up to 45
	// seconds for the swarm service to converge before it answers, so a
	// libsql.deploy on the rig ran past a 60-second cap while the server
	// finished the deploy (Phase 3 probe, 2026-09-05). The deadline lives on
	// the request context, not on http.Client.Timeout, so the two can differ.
	requestTimeout time.Duration
	deployTimeout  time.Duration
}

type Option func(*Client)

// WithRetryBaseWait overrides the retry backoff base; used by tests.
func WithRetryBaseWait(d time.Duration) Option {
	return func(c *Client) { c.retryBaseWait = d }
}

// WithRequestTimeout overrides the per-attempt deadline of every call except
// the deploy endpoints; used by tests.
func WithRequestTimeout(d time.Duration) Option {
	return func(c *Client) { c.requestTimeout = d }
}

// WithDeployTimeout overrides the per-attempt deadline of the synchronous
// *.deploy endpoints; used by tests.
func WithDeployTimeout(d time.Duration) Option {
	return func(c *Client) { c.deployTimeout = d }
}

func New(endpoint, apiKey string, insecure bool, version string, opts ...Option) (*Client, error) {
	endpoint = strings.TrimRight(endpoint, "/")
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint must not be empty")
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		return nil, fmt.Errorf("endpoint must start with http:// or https://, got %q", endpoint)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	c := &Client{
		endpoint:       endpoint,
		apiKey:         apiKey,
		http:           &http.Client{Transport: transport},
		userAgent:      "terraform-provider-dokploy/" + version,
		retryAttempts:  3,
		retryBaseWait:  time.Second,
		requestTimeout: 60 * time.Second,
		deployTimeout:  10 * time.Minute,
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// Get calls a tRPC query endpoint; inputs go in the query string.
// Idempotent, so transient failures are retried.
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, out, c.requestTimeout)
}

// Post calls a tRPC mutation endpoint; inputs go in the JSON body.
// Never retried.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, nil, body, out, c.requestTimeout)
}

// PostDeploy calls one of the synchronous *.deploy mutations, which answer
// only after the server has pulled, built, and waited for the swarm service
// to converge. It uses the longer deployTimeout; see the Client field comment.
func (c *Client) PostDeploy(ctx context.Context, path string, body any) error {
	return c.do(ctx, http.MethodPost, path, nil, body, nil, c.deployTimeout)
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any, timeout time.Duration) error {
	u := c.endpoint + basePath + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("%s %s: encoding request: %w", method, path, err)
		}
	}
	attempts := 1
	if method == http.MethodGet {
		attempts = c.retryAttempts
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.retryBaseWait << (i - 1)): // 1x, 2x, 4x...
			}
		}
		var retryable bool
		retryable, lastErr = c.attempt(ctx, method, u, path, payload, out, timeout)
		if lastErr == nil || !retryable {
			return lastErr
		}
	}
	return lastErr
}

// attempt performs one round-trip and reports whether a failure is
// retryable (network error or 5xx).
func (c *Client) attempt(ctx context.Context, method, u, path string, payload []byte, out any, timeout time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var bodyReader io.Reader
	if payload != nil {
		bodyReader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return false, err
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("User-Agent", c.userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return true, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return true, fmt.Errorf("%s %s: reading response: %w", method, path, err)
	}
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if out == nil || len(bytes.TrimSpace(raw)) == 0 {
			return false, nil
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return false, fmt.Errorf("%s %s: decoding response: %w", method, path, err)
		}
		return false, nil
	case resp.StatusCode == http.StatusNotFound:
		return false, fmt.Errorf("%s %s: %w", method, path, ErrNotFound)
	default:
		return resp.StatusCode >= 500, apiError(method, path, resp.StatusCode, raw)
	}
}

// apiError parses the tRPC error envelope, degrading to the raw body.
func apiError(method, path string, status int, raw []byte) error {
	var envelope struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	_ = json.Unmarshal(raw, &envelope)
	if envelope.Message == "" {
		envelope.Message = strings.TrimSpace(string(raw))
	}
	return &DokployError{
		Code:       envelope.Code,
		Message:    envelope.Message,
		HTTPStatus: status,
		Method:     method,
		Path:       path,
	}
}

// Endpoint returns the base URL the client was configured with, without a
// trailing slash. The git provider resources derive the OAuth redirect URI
// from it when the configuration leaves the attribute unset.
func (c *Client) Endpoint() string { return strings.TrimRight(c.endpoint, "/") }
