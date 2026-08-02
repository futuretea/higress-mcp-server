// Package console provides a small, version-tolerant client for Higress Console.
package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

const maxResponseBytes = 8 << 20

// Config configures a Console API client.
type Config struct {
	BaseURL        string
	Username       string
	Password       string
	AuthMode       string
	TimeoutSeconds int
}

// Request describes one Console API request.
type Request struct {
	Method string
	Path   string
	Query  map[string]string
	Body   any
}

// Response is the Console response without credential-bearing headers.
type Response struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

// Client sends requests only to one configured Higress Console origin.
type Client struct {
	baseURL  *url.URL
	username string
	password string
	authMode string
	http     *http.Client

	authMu        sync.Mutex
	authenticated bool
	authAttempt   *authenticationAttempt
}

type authenticationAttempt struct {
	done chan struct{}
	err  error
}

// NewClient validates configuration and creates a Console client.
func NewClient(cfg Config) (*Client, error) {
	baseURL, err := parseBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	if (cfg.Username == "") != (cfg.Password == "") {
		return nil, errors.New("console username and password must be set together")
	}
	authMode := strings.ToLower(strings.TrimSpace(cfg.AuthMode))
	if authMode == "" {
		authMode = "session"
	}
	if authMode != "session" && authMode != "basic" {
		return nil, fmt.Errorf("unsupported console auth mode %q", cfg.AuthMode)
	}
	if cfg.TimeoutSeconds < 1 || cfg.TimeoutSeconds > 300 {
		return nil, fmt.Errorf("console timeout must be between 1 and 300 seconds, got %d", cfg.TimeoutSeconds)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}
	return &Client{
		baseURL:  baseURL,
		username: cfg.Username,
		password: cfg.Password,
		authMode: authMode,
		http: &http.Client{
			Timeout:       time.Duration(cfg.TimeoutSeconds) * time.Second,
			Jar:           jar,
			CheckRedirect: rejectCrossOriginRedirect(baseURL),
		},
	}, nil
}

// Request sends a request to Higress Console and returns its unmodified body.
func (c *Client) Request(ctx context.Context, request Request) (Response, error) {
	if c == nil {
		return Response{}, errors.New("console client is required")
	}
	if err := c.ensureAuthenticated(ctx); err != nil {
		return Response{}, err
	}
	return c.do(ctx, request)
}

func (c *Client) ensureAuthenticated(ctx context.Context) error {
	if c.username == "" || c.authMode == "basic" {
		return nil
	}

	c.authMu.Lock()
	if c.authenticated {
		c.authMu.Unlock()
		return nil
	}
	if attempt := c.authAttempt; attempt != nil {
		c.authMu.Unlock()
		select {
		case <-attempt.done:
			if attempt.err != nil {
				return fmt.Errorf("authenticate with Higress Console: %w", attempt.err)
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	attempt := &authenticationAttempt{done: make(chan struct{})}
	c.authAttempt = attempt
	c.authMu.Unlock()

	_, err := c.do(ctx, Request{
		Method: http.MethodPost,
		Path:   "/session/login",
		Body: map[string]string{
			"username": c.username,
			"password": c.password,
		},
	})
	c.authMu.Lock()
	attempt.err = err
	if err == nil {
		c.authenticated = true
	}
	close(attempt.done)
	if c.authAttempt == attempt {
		c.authAttempt = nil
	}
	c.authMu.Unlock()
	if err != nil {
		return fmt.Errorf("authenticate with Higress Console: %w", err)
	}
	return nil
}

func (c *Client) do(ctx context.Context, request Request) (Response, error) {
	method, err := normalizeMethod(request.Method)
	if err != nil {
		return Response{}, err
	}
	endpoint, err := c.resolvePath(request.Path, request.Query)
	if err != nil {
		return Response{}, err
	}

	var body io.Reader
	if request.Body != nil {
		payload, err := json.Marshal(request.Body)
		if err != nil {
			return Response{}, fmt.Errorf("encode Console request body: %w", err)
		}
		body = bytes.NewReader(payload)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return Response{}, fmt.Errorf("create Console request: %w", err)
	}
	if request.Body != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	httpRequest.Header.Set("Accept", "application/json, text/plain;q=0.9, */*;q=0.8")
	if c.authMode == "basic" && c.username != "" {
		httpRequest.SetBasicAuth(c.username, c.password)
	}

	httpResponse, err := c.http.Do(httpRequest)
	if err != nil {
		return Response{}, fmt.Errorf("request Higress Console: %w", err)
	}
	defer func() { _ = httpResponse.Body.Close() }()

	responseBody, err := readResponseBody(httpResponse.Body)
	if err != nil {
		return Response{}, err
	}
	response := Response{
		StatusCode:  httpResponse.StatusCode,
		ContentType: httpResponse.Header.Get("Content-Type"),
		Body:        responseBody,
	}
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		return response, fmt.Errorf("higress Console returned HTTP %d: %s", httpResponse.StatusCode, summarizeBody(responseBody))
	}
	return response, nil
}

func readResponseBody(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Higress Console response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return nil, fmt.Errorf("higress Console response exceeds %d MiB", maxResponseBytes>>20)
	}
	return data, nil
}

func summarizeBody(body []byte) string {
	const maxSummaryBytes = 1024
	text := string(bytes.TrimSpace(body))
	if len(text) > maxSummaryBytes {
		return text[:maxSummaryBytes] + "…"
	}
	return text
}

func rejectCrossOriginRedirect(baseURL *url.URL) func(*http.Request, []*http.Request) error {
	return func(request *http.Request, _ []*http.Request) error {
		if sameOrigin(request.URL, baseURL) && isWithinBasePath(request.URL.Path, baseURL.Path) && !hasTraversalSegment(request.URL.Path) {
			return nil
		}
		return fmt.Errorf("console redirect leaves configured base URL: %s", request.URL.Redacted())
	}
}
