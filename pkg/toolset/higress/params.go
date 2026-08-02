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

func formatJSON(value any) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", fmt.Errorf("format tool response: %w", err)
	}
	return string(data), nil
}
