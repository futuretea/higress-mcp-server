package higress

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

func redactConsumerKeyAuthCredentials(response console.Response) console.Response {
	if !json.Valid(response.Body) {
		return response
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(response.Body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return response
	}
	if !redactConsumerData(payload["data"]) {
		return response
	}
	redacted, err := json.Marshal(payload)
	if err != nil {
		return response
	}
	response.Body = redacted
	return response
}

func redactedConsumerReadError(response console.Response) error {
	const maxSummaryBytes = 1024

	summary := strings.TrimSpace(string(response.Body))
	if len(summary) > maxSummaryBytes {
		summary = summary[:maxSummaryBytes] + "…"
	}
	return fmt.Errorf("higress Console returned HTTP %d: %s", response.StatusCode, summary)
}

func redactConsumerData(data any) bool {
	switch consumers := data.(type) {
	case map[string]any:
		return redactConsumerCredentials(consumers)
	case []any:
		redacted := false
		for _, consumer := range consumers {
			consumer, ok := consumer.(map[string]any)
			if ok && redactConsumerCredentials(consumer) {
				redacted = true
			}
		}
		return redacted
	default:
		return false
	}
}

func redactConsumerCredentials(consumer map[string]any) bool {
	credentials, ok := consumer["credentials"].([]any)
	if !ok {
		return false
	}

	redacted := false
	for _, credential := range credentials {
		credential, ok := credential.(map[string]any)
		if !ok || credential["type"] != "key-auth" {
			continue
		}
		values, ok := credential["values"].([]any)
		if !ok {
			continue
		}
		for index := range values {
			values[index] = "***"
		}
		redacted = true
	}
	return redacted
}

func formatJSON(value any) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", fmt.Errorf("format tool response: %w", err)
	}
	return string(data), nil
}
