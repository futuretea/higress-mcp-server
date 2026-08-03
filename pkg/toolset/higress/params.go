package higress

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/futuretea/higress-mcp-server/pkg/console"
)

func requiredString(params map[string]any, name string) (string, error) {
	value, ok := optionalString(params, name)
	if !ok {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func optionalString(params map[string]any, name string) (string, bool) {
	value, ok := params[name].(string)
	value = strings.TrimSpace(value)
	return value, ok && value != ""
}

func optionalValue(params map[string]any, name string) string {
	value, _ := optionalString(params, name)
	return value
}

func optionalBool(params map[string]any, name string) (bool, error) {
	value, ok := params[name]
	if !ok || value == nil {
		return false, nil
	}
	result, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return result, nil
}

func requiredObject(params map[string]any, name string) (map[string]any, error) {
	value, ok := params[name].(map[string]any)
	if !ok || value == nil {
		return nil, fmt.Errorf("%s must be a JSON object", name)
	}
	return value, nil
}

func stringMap(value any) (map[string]string, error) {
	if value == nil {
		return nil, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("must be a JSON object with string values")
	}
	result := make(map[string]string, len(object))
	for key, item := range object {
		stringValue, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%q must be a string", key)
		}
		result[key] = stringValue
	}
	return result, nil
}

func formatResponse(response console.Response) string {
	if json.Valid(response.Body) {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, response.Body, "", "  "); err == nil {
			return pretty.String()
		}
	}
	return fmt.Sprintf("HTTP %d (%s)\n%s", response.StatusCode, response.ContentType, string(response.Body))
}

func redactSensitiveResponse(request console.Request, response console.Response) (console.Response, bool, error) {
	kind := sensitiveResponseKindForPath(request.Path)
	if kind == sensitiveResponseNone {
		return response, false, nil
	}
	if !json.Valid(response.Body) {
		return console.Response{}, false, unredactableSensitiveResponseError()
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(response.Body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		return console.Response{}, false, unredactableSensitiveResponseError()
	}
	data, ok := payload["data"]
	if !ok {
		return console.Response{}, false, unredactableSensitiveResponseError()
	}
	switch redactSensitiveData(kind, data) {
	case sensitiveDataAbsent:
		return response, false, nil
	case sensitiveDataUnsupported:
		return console.Response{}, false, unredactableSensitiveResponseError()
	}
	redacted, err := json.Marshal(payload)
	if err != nil {
		return console.Response{}, false, unredactableSensitiveResponseError()
	}
	response.Body = redacted
	return response, true, nil
}

func unredactableSensitiveResponseError() error {
	return errors.New("cannot safely process sensitive Higress Console response")
}

func redactedConsoleResponseError(response console.Response) error {
	const maxSummaryBytes = 1024

	summary := strings.TrimSpace(string(response.Body))
	if len(summary) > maxSummaryBytes {
		summary = summary[:maxSummaryBytes] + "…"
	}
	return fmt.Errorf("higress Console returned HTTP %d: %s", response.StatusCode, summary)
}

type sensitiveResponseKind uint8

const (
	sensitiveResponseNone sensitiveResponseKind = iota
	sensitiveResponseConsumer
	sensitiveResponseTLSCertificate
	sensitiveResponseProvider
	sensitiveResponseMCPServer
	sensitiveResponseUser
)

type sensitiveDataResult uint8

const (
	sensitiveDataAbsent sensitiveDataResult = iota
	sensitiveDataRedacted
	sensitiveDataUnsupported
)

func sensitiveResponseKindForPath(rawPath string) sensitiveResponseKind {
	parsed, err := url.Parse(rawPath)
	if err != nil {
		return sensitiveResponseNone
	}
	switch {
	case isResourceResponsePath(parsed.Path, "/v1/consumers"):
		return sensitiveResponseConsumer
	case isResourceResponsePath(parsed.Path, "/v1/tls-certificates"):
		return sensitiveResponseTLSCertificate
	case isResourceResponsePath(parsed.Path, "/v1/ai/providers"):
		return sensitiveResponseProvider
	case isResourceResponsePath(parsed.Path, "/v1/mcpServer"):
		return sensitiveResponseMCPServer
	case parsed.Path == "/user/info":
		return sensitiveResponseUser
	default:
		return sensitiveResponseNone
	}
}

func isResourceResponsePath(path, collection string) bool {
	path = strings.TrimRight(path, "/")
	if path == collection {
		return true
	}
	name := strings.TrimPrefix(path, collection+"/")
	return name != path && name != "" && !strings.Contains(name, "/")
}

func redactSensitiveData(kind sensitiveResponseKind, data any) sensitiveDataResult {
	switch items := data.(type) {
	case map[string]any:
		return redactSensitiveItem(kind, items)
	case []any:
		result := sensitiveDataAbsent
		for _, item := range items {
			item, ok := item.(map[string]any)
			if !ok {
				return sensitiveDataUnsupported
			}
			switch redactSensitiveItem(kind, item) {
			case sensitiveDataUnsupported:
				return sensitiveDataUnsupported
			case sensitiveDataRedacted:
				result = sensitiveDataRedacted
			}
		}
		return result
	default:
		return sensitiveDataUnsupported
	}
}

func redactSensitiveItem(kind sensitiveResponseKind, item map[string]any) sensitiveDataResult {
	switch kind {
	case sensitiveResponseConsumer:
		return redactConsumerCredentials(item)
	case sensitiveResponseTLSCertificate:
		return redactValue(item, "key")
	case sensitiveResponseProvider:
		return redactValues(item, "tokens")
	case sensitiveResponseMCPServer:
		return redactMCPServerPassword(item)
	case sensitiveResponseUser:
		return redactValue(item, "password")
	default:
		return sensitiveDataAbsent
	}
}

func redactConsumerCredentials(consumer map[string]any) sensitiveDataResult {
	value, ok := consumer["credentials"]
	if !ok {
		return sensitiveDataAbsent
	}
	credentials, ok := value.([]any)
	if !ok {
		return sensitiveDataUnsupported
	}

	result := sensitiveDataAbsent
	for _, credential := range credentials {
		credential, ok := credential.(map[string]any)
		if !ok {
			return sensitiveDataUnsupported
		}
		if credential["type"] != "key-auth" {
			continue
		}
		switch redactValues(credential, "values") {
		case sensitiveDataUnsupported:
			return sensitiveDataUnsupported
		case sensitiveDataRedacted:
			result = sensitiveDataRedacted
		}
	}
	return result
}

func redactMCPServerPassword(item map[string]any) sensitiveDataResult {
	value, ok := item["dbConfig"]
	if !ok {
		return sensitiveDataAbsent
	}
	config, ok := value.(map[string]any)
	if !ok {
		return sensitiveDataUnsupported
	}
	return redactValue(config, "password")
}

func redactValues(item map[string]any, field string) sensitiveDataResult {
	value, ok := item[field]
	if !ok {
		return sensitiveDataAbsent
	}
	values, ok := value.([]any)
	if !ok {
		return sensitiveDataUnsupported
	}
	for index := range values {
		values[index] = "***"
	}
	return sensitiveDataRedacted
}

func redactValue(item map[string]any, field string) sensitiveDataResult {
	if _, ok := item[field]; !ok {
		return sensitiveDataAbsent
	}
	item[field] = "***"
	return sensitiveDataRedacted
}

func formatJSON(value any) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", fmt.Errorf("format tool response: %w", err)
	}
	return string(data), nil
}
