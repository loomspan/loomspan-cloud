package invocation

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
)

type Response struct {
	StatusCode  int
	SafeToRetry bool
	Body        ResponseBody
}

// Raw diagnostic fields preserve complete JSON and explicitly represent an unavailable view.
type ResponseBody struct {
	Result        *string         `json:"result,omitempty"`
	Message       json.RawMessage `json:"message,omitempty"`
	Issues        json.RawMessage `json:"issues,omitempty"`
	Debug         json.RawMessage `json:"debug,omitempty"`
	Diagnosis     json.RawMessage `json:"diagnosis,omitempty"`
	ExecutionView json.RawMessage `json:"executionView,omitempty"`
}

func projectResponse(response *http.Response, showExecution bool) (Response, error) {
	result := Response{StatusCode: response.StatusCode, SafeToRetry: response.StatusCode == http.StatusServiceUnavailable}
	if showExecution {
		result.Body.ExecutionView = json.RawMessage("null")
	}
	fields, err := readResponse(response)
	if err != nil && !result.SafeToRetry {
		return Response{}, ErrOutcomeUnknown
	}
	if response.StatusCode == http.StatusOK {
		if err := json.Unmarshal(fields["result"], &result.Body.Result); err != nil || result.Body.Result == nil {
			return Response{}, ErrOutcomeUnknown
		}
	} else if response.StatusCode >= 400 {
		if isStringOrNull(fields["message"]) {
			result.Body.Message = fields["message"]
		} else if result.SafeToRetry {
			result.Body.Message = json.RawMessage(`"engine rejected invocation before execution"`)
		} else {
			return Response{}, ErrOutcomeUnknown
		}
		result.Body.Issues = fields["issues"]
		if showExecution {
			result.Body.Debug = fields["debug"]
			result.Body.Diagnosis = fields["diagnosis"]
		}
	} else {
		return Response{}, ErrOutcomeUnknown
	}
	if showExecution && len(fields["executionView"]) != 0 {
		result.Body.ExecutionView = fields["executionView"]
	}
	return result, nil
}

func readResponse(response *http.Response) (map[string]json.RawMessage, error) {
	media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if media != "application/json" {
		return nil, ErrOutcomeUnknown
	}
	decoder := json.NewDecoder(response.Body)
	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return nil, ErrOutcomeUnknown
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, ErrOutcomeUnknown
	}
	return fields, nil
}

func isStringOrNull(raw json.RawMessage) bool {
	return len(raw) > 0 && (raw[0] == '"' || bytes.Equal(raw, []byte("null")))
}
