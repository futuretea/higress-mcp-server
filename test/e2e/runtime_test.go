//go:build !short

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func startHigress(t *testing.T, ctx context.Context, nameSuffix, version string) (string, string, string) {
	t.Helper()
	backendName := "higress-mcp-e2e-backend-" + nameSuffix
	backend := startContainer(t, ctx, backendName, backendImage, nil, []string{"-listen=:5678", "-text=" + backendResponse})
	t.Cleanup(func() { backend.stop(t) })

	backendIP := inspectContainerIP(t, ctx, backendName)
	higressName := "higress-mcp-e2e-" + nameSuffix
	higress := startContainer(t, ctx, higressName, e2eImageRepository+":"+version,
		[]string{"-p", "127.0.0.1::8001", "-p", "127.0.0.1::8080"}, nil)
	t.Cleanup(func() { higress.stop(t) })

	consoleURL := "http://" + containerPort(t, ctx, higressName, "8001/tcp")
	gatewayURL := "http://" + containerPort(t, ctx, higressName, "8080/tcp")
	waitForHTTP(t, ctx, consoleURL+"/system/info", func(response *http.Response, _ []byte) bool {
		return response.StatusCode == http.StatusOK
	})
	return consoleURL, gatewayURL, backendIP
}

type runningContainer struct {
	name string
}

func startContainer(t *testing.T, ctx context.Context, name, image string, dockerArgs, containerArgs []string) runningContainer {
	t.Helper()
	command := append([]string{"run", "-d", "--rm", "--name", name}, dockerArgs...)
	command = append(command, image)
	command = append(command, containerArgs...)
	if output, err := runCommand(ctx, "docker", command...); err != nil {
		t.Fatalf("start Docker container %q: %v\n%s", name, err, output)
	}
	return runningContainer{name: name}
}

func (c runningContainer) stop(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if output, err := runCommand(ctx, "docker", "stop", "--time", "10", c.name); err != nil && !strings.Contains(output, "No such container") {
		t.Logf("stop Docker container %q: %v\n%s", c.name, err, output)
	}
}

func inspectContainerIP(t *testing.T, ctx context.Context, name string) string {
	t.Helper()
	output, err := runCommand(ctx, "docker", "inspect", "--format", "{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}", name)
	if err != nil {
		t.Fatalf("inspect backend container %q: %v\n%s", name, err, output)
	}
	ip := strings.TrimSpace(output)
	if net.ParseIP(ip) == nil {
		t.Fatalf("backend container %q has invalid IP %q", name, ip)
	}
	return ip
}

func containerPort(t *testing.T, ctx context.Context, name, containerPort string) string {
	t.Helper()
	output, err := runCommand(ctx, "docker", "port", name, containerPort)
	if err != nil {
		t.Fatalf("read Docker port for %q: %v\n%s", name, err, output)
	}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if _, _, err := net.SplitHostPort(line); err == nil {
			return line
		}
	}
	t.Fatalf("Docker did not report a usable port for %q: %q", name, output)
	return ""
}

type runningProcess struct {
	command *exec.Cmd
	done    chan error
	output  *bytes.Buffer
}

func startMCPServer(t *testing.T, ctx context.Context, binary, consoleURL, username, password string) string {
	t.Helper()
	port := reservePort(t)
	processCtx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(processCtx, binary, "mcp", "--port", strconv.Itoa(port), "--listen", "127.0.0.1", "--console-url", consoleURL)
	output := &bytes.Buffer{}
	command.Stdout = output
	command.Stderr = output
	command.Env = withEnvironment(os.Environ(), map[string]string{
		"MCP_CONSOLE_URL":      consoleURL,
		"MCP_CONSOLE_USERNAME": username,
		"MCP_CONSOLE_PASSWORD": password,
	})
	if err := command.Start(); err != nil {
		cancel()
		t.Fatalf("start MCP server: %v", err)
	}
	process := &runningProcess{command: command, done: make(chan error, 1), output: output}
	go func() { process.done <- command.Wait() }()
	t.Cleanup(cancel)
	t.Cleanup(func() { process.stop(t) })
	endpoint := "http://127.0.0.1:" + strconv.Itoa(port)
	waitForHTTP(t, ctx, endpoint+"/healthz", func(response *http.Response, _ []byte) bool {
		return response.StatusCode == http.StatusOK
	})
	return endpoint
}

func (p *runningProcess) stop(t *testing.T) {
	t.Helper()
	if p.command.Process == nil {
		return
	}
	_ = p.command.Process.Signal(os.Interrupt)
	select {
	case err := <-p.done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("MCP server exited during cleanup: %v\n%s", err, p.output.String())
		}
	case <-time.After(10 * time.Second):
		_ = p.command.Process.Kill()
		<-p.done
	}
}

func reservePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve local MCP port: %v", err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForHTTP(t *testing.T, ctx context.Context, endpoint string, ready func(*http.Response, []byte) bool) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	var last string
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			t.Fatalf("build request for %s: %v", endpoint, err)
		}
		response, err := client.Do(request)
		if err == nil {
			body, readErr := io.ReadAll(io.LimitReader(response.Body, 64<<10))
			_ = response.Body.Close()
			if readErr == nil && ready(response, body) {
				return
			}
			last = fmt.Sprintf("status=%d body=%q readErr=%v", response.StatusCode, body, readErr)
		} else {
			last = err.Error()
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for %s: %v (last response: %s)", endpoint, ctx.Err(), last)
		case <-time.After(2 * time.Second):
		}
	}
}

func postJSON(t *testing.T, ctx context.Context, endpoint string, payload any) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode POST %s: %v", endpoint, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build POST %s: %v", endpoint, err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer func() { _ = response.Body.Close() }()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		t.Fatalf("POST %s returned HTTP %d: %s", endpoint, response.StatusCode, responseBody)
	}
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	return string(output), err
}

func withEnvironment(base []string, values map[string]string) []string {
	result := make([]string, 0, len(base)+len(values))
	for _, item := range base {
		key, _, found := strings.Cut(item, "=")
		if !found || values[key] == "" {
			result = append(result, item)
		}
	}
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}
