package console_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/futuretea/higress-mcp-server/pkg/console"
)

func TestClientAuthenticatesAndRequestsConsole(t *testing.T) {
	loginCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/console/session/login":
			loginCalls++
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode login: %v", err)
			}
			if body["username"] != "admin" || body["password"] != "secret" {
				t.Fatalf("unexpected login body: %#v", body)
			}
			http.SetCookie(w, &http.Cookie{Name: "HIGRESS_SESSION", Value: "session", Path: "/"})
			_, _ = w.Write([]byte(`{"code":0}`))
		case "/console/v1/routes":
			if _, err := r.Cookie("HIGRESS_SESSION"); err != nil {
				t.Fatalf("missing authenticated cookie: %v", err)
			}
			if got := r.URL.Query().Get("page"); got != "2" {
				t.Fatalf("page = %q, want 2", got)
			}
			_, _ = w.Write([]byte(`{"data":["route-a"]}`))
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := console.NewClient(console.Config{
		BaseURL:        server.URL + "/console",
		Username:       "admin",
		Password:       "secret",
		TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	for range 2 {
		response, err := client.Request(context.Background(), console.Request{
			Method: http.MethodGet,
			Path:   "/v1/routes",
			Query:  map[string]string{"page": "2"},
		})
		if err != nil {
			t.Fatalf("Request: %v", err)
		}
		if response.StatusCode != http.StatusOK || string(response.Body) != `{"data":["route-a"]}` {
			t.Fatalf("unexpected response: %#v", response)
		}
	}
	if loginCalls != 1 {
		t.Fatalf("login calls = %d, want 1", loginCalls)
	}
}

func TestClientHonorsWaitingRequestContextDuringSessionLogin(t *testing.T) {
	loginStarted := make(chan struct{})
	allowLogin := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session/login":
			close(loginStarted)
			select {
			case <-allowLogin:
				http.SetCookie(w, &http.Cookie{Name: "HIGRESS_SESSION", Value: "session", Path: "/"})
				_, _ = w.Write([]byte(`{"code":0}`))
			case <-r.Context().Done():
			}
		case "/v1/routes":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := console.NewClient(console.Config{
		BaseURL: server.URL, Username: "admin", Password: "secret", TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	firstDone := make(chan error, 1)
	go func() {
		_, requestErr := client.Request(context.Background(), console.Request{Method: http.MethodGet, Path: "/v1/routes"})
		firstDone <- requestErr
	}()
	select {
	case <-loginStarted:
	case <-time.After(time.Second):
		t.Fatal("session login did not start")
	}

	requestCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = client.Request(requestCtx, console.Request{Method: http.MethodGet, Path: "/v1/routes"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting request error = %v, want context deadline exceeded", err)
	}

	close(allowLogin)
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("initial request: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("initial request did not finish after login completed")
	}
}

func TestClientRejectsEscapingConfiguredConsole(t *testing.T) {
	client, err := console.NewClient(console.Config{BaseURL: "https://console.example", TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	for _, path := range []string{
		"https://other.example/v1/routes",
		"/v1/routes?x=1",
		"/v1/../session/login",
		"/%2e%2e%2fadmin",
		"/v1%2f..%2fadmin",
	} {
		_, err := client.Request(context.Background(), console.Request{Method: http.MethodGet, Path: path})
		if err == nil || !strings.Contains(err.Error(), "console API path") {
			t.Fatalf("Request(%q) error = %v, want path validation error", path, err)
		}
	}
}

func TestClientKeepsCredentialsWithinConfiguredPathPrefix(t *testing.T) {
	for _, authMode := range []string{"session", "basic"} {
		t.Run(authMode, func(t *testing.T) {
			paths := make([]string, 0, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				switch r.URL.Path {
				case "/console/session/login":
					http.SetCookie(w, &http.Cookie{Name: "HIGRESS_SESSION", Value: "session", Path: "/"})
					_, _ = w.Write([]byte(`{"code":0}`))
				case "/console/v1/routes":
					if authMode == "basic" {
						username, password, ok := r.BasicAuth()
						if !ok || username != "admin" || password != "secret" {
							t.Errorf("unexpected basic authentication: %q, %q, %t", username, password, ok)
						}
					} else if _, err := r.Cookie("HIGRESS_SESSION"); err != nil {
						t.Errorf("missing Console session cookie: %v", err)
					}
					_, _ = w.Write([]byte(`{"data":[]}`))
				default:
					t.Errorf("unexpected request path %q", r.URL.Path)
				}
			}))
			defer server.Close()

			client, err := console.NewClient(console.Config{
				BaseURL: server.URL + "/console", Username: "admin", Password: "secret", AuthMode: authMode, TimeoutSeconds: 5,
			})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			if _, err := client.Request(context.Background(), console.Request{Method: http.MethodGet, Path: "/v1/routes"}); err != nil {
				t.Fatalf("valid request: %v", err)
			}
			for _, path := range []string{"/%2e%2e%2fadmin", "/v1%2f..%2fadmin"} {
				if _, err := client.Request(context.Background(), console.Request{Method: http.MethodGet, Path: path}); err == nil || !strings.Contains(err.Error(), "traversal") {
					t.Fatalf("Request(%q) error = %v, want traversal rejection", path, err)
				}
			}
			for _, path := range paths {
				if !strings.HasPrefix(path, "/console/") {
					t.Fatalf("request escaped Console path prefix: %q", path)
				}
			}
		})
	}
}

func TestClientReportsConsoleError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"invalid route"}`))
	}))
	defer server.Close()

	client, err := console.NewClient(console.Config{BaseURL: server.URL, TimeoutSeconds: 5})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.Request(context.Background(), console.Request{Method: http.MethodGet, Path: "/v1/routes"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") || !strings.Contains(err.Error(), "invalid route") {
		t.Fatalf("Request error = %v", err)
	}
}

func TestClientUsesBasicAuthenticationWithoutSessionLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session/login" {
			t.Fatal("basic mode must not request a session login")
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "admin" || password != "secret" {
			t.Fatalf("unexpected basic authentication: %q, %q, %t", username, password, ok)
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	client, err := console.NewClient(console.Config{
		BaseURL: server.URL, Username: "admin", Password: "secret", AuthMode: "basic", TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.Request(context.Background(), console.Request{Method: http.MethodGet, Path: "/v1/routes"}); err != nil {
		t.Fatalf("Request: %v", err)
	}
}

func TestClientRejectsCrossOriginRedirectBeforeSendingCredentials(t *testing.T) {
	for _, authMode := range []string{"session", "basic"} {
		t.Run(authMode, func(t *testing.T) {
			targetCalls := 0
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				targetCalls++
				if _, _, ok := r.BasicAuth(); ok {
					t.Error("redirect target received Basic authentication")
				}
				if _, err := r.Cookie("HIGRESS_SESSION"); err == nil {
					t.Error("redirect target received Console session cookie")
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer target.Close()

			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/session/login" {
					http.SetCookie(w, &http.Cookie{Name: "HIGRESS_SESSION", Value: "session", Path: "/"})
					_, _ = w.Write([]byte(`{"code":0}`))
					return
				}
				http.Redirect(w, r, target.URL+"/v1/routes", http.StatusFound)
			}))
			defer origin.Close()

			client, err := console.NewClient(console.Config{
				BaseURL: origin.URL, Username: "admin", Password: "secret", AuthMode: authMode, TimeoutSeconds: 5,
			})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			_, err = client.Request(context.Background(), console.Request{Method: http.MethodGet, Path: "/v1/routes"})
			if err == nil || !strings.Contains(err.Error(), "redirect leaves configured base URL") {
				t.Fatalf("Request error = %v, want cross-origin redirect rejection", err)
			}
			if targetCalls != 0 {
				t.Fatalf("redirect target calls = %d, want 0", targetCalls)
			}
		})
	}
}

func TestClientRejectsRedirectOutsideBasePathBeforeSendingCredentials(t *testing.T) {
	for _, redirectPath := range []string{"/other-app", "/console/%2e%2e/other-app"} {
		t.Run(redirectPath, func(t *testing.T) {
			for _, authMode := range []string{"session", "basic"} {
				t.Run(authMode, func(t *testing.T) {
					targetCalls := 0
					origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						switch r.URL.Path {
						case "/console/session/login":
							http.SetCookie(w, &http.Cookie{Name: "HIGRESS_SESSION", Value: "session", Path: "/"})
							_, _ = w.Write([]byte(`{"code":0}`))
						case "/console/v1/routes":
							w.Header().Set("Location", redirectPath)
							w.WriteHeader(http.StatusFound)
						default:
							targetCalls++
							if _, _, ok := r.BasicAuth(); ok {
								t.Error("redirect target received Basic authentication")
							}
							if _, err := r.Cookie("HIGRESS_SESSION"); err == nil {
								t.Error("redirect target received Console session cookie")
							}
							w.WriteHeader(http.StatusNoContent)
						}
					}))
					defer origin.Close()

					client, err := console.NewClient(console.Config{
						BaseURL: origin.URL + "/console", Username: "admin", Password: "secret", AuthMode: authMode, TimeoutSeconds: 5,
					})
					if err != nil {
						t.Fatalf("NewClient: %v", err)
					}
					_, err = client.Request(context.Background(), console.Request{Method: http.MethodGet, Path: "/v1/routes"})
					if err == nil || !strings.Contains(err.Error(), "redirect leaves configured base URL") {
						t.Fatalf("Request error = %v, want base-path redirect rejection", err)
					}
					if targetCalls != 0 {
						t.Fatalf("redirect target calls = %d, want 0", targetCalls)
					}
				})
			}
		})
	}
}
