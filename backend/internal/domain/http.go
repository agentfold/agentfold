package domain

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// HTTP request modes: wait for the response and leave by success or
// failed, or fire the request and go on by next right away.
const (
	HTTPModeWait          = "wait"
	HTTPModeFireAndForget = "fire_and_forget"
)

// Retry backoffs: the same delay between attempts, or doubling it.
const (
	BackoffFixed       = "fixed"
	BackoffExponential = "exponential"
)

// Defaults and bounds of HTTPRequestConfig.
const (
	DefaultHTTPTimeout    = 30 * time.Second
	MaxHTTPTimeout        = 5 * time.Minute
	DefaultRetryDelay     = 30 * time.Second
	MaxRetryDelay         = time.Hour
	MaxHTTPAttempts       = 20
	MaxHTTPResponseBody   = 1 << 20
	httpTimeoutSecondsMax = int(MaxHTTPTimeout / time.Second)
	retryDelaySecondsMax  = int(MaxRetryDelay / time.Second)
)

// HTTPOutputs are the outputs of an http_request node in fire_and_forget
// mode; in wait mode they are Outputs[NodeHTTPRequest].
var HTTPOutputs = []string{"next"}

var httpMethods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead}

// HTTPRequestConfig calls an HTTP endpoint. URL, header values and body
// take placeholders (see RenderTemplate). In wait mode the node leaves by
// success when the response status is one of SuccessStatuses (2xx when
// empty) and by failed otherwise, after MaxAttempts attempts for network
// errors, timeouts, 429 and 5xx responses.
type HTTPRequestConfig struct {
	Method          string       `json:"method,omitempty"`
	URL             string       `json:"url,omitempty"`
	Headers         []HTTPHeader `json:"headers,omitempty"`
	Body            string       `json:"body,omitempty"`
	Mode            string       `json:"mode,omitempty"`
	TimeoutSeconds  int          `json:"timeout_seconds,omitempty"`
	SuccessStatuses []int        `json:"success_statuses,omitempty"`
	MaxAttempts     int          `json:"max_attempts,omitempty"`
	// RetryDelaySeconds is the wait before the second attempt; with the
	// exponential backoff it doubles after each one.
	RetryDelaySeconds int    `json:"retry_delay_seconds,omitempty"`
	Backoff           string `json:"backoff,omitempty"`
}

// HTTPHeader is a request header. The value of a secret header is sealed
// into Sealed when the draft is saved; Value is then empty.
type HTTPHeader struct {
	Name   string `json:"name"`
	Value  string `json:"value,omitempty"`
	Secret bool   `json:"secret,omitempty"`
	Sealed []byte `json:"sealed,omitempty"`
}

// FireAndForget tells whether the node goes on without waiting.
func (c HTTPRequestConfig) FireAndForget() bool { return c.Mode == HTTPModeFireAndForget }

// Timeout bounds each attempt.
func (c HTTPRequestConfig) Timeout() time.Duration {
	if c.TimeoutSeconds <= 0 {
		return DefaultHTTPTimeout
	}
	return time.Duration(c.TimeoutSeconds) * time.Second
}

// Attempts is how many times the request is tried: once when firing and
// forgetting.
func (c HTTPRequestConfig) Attempts() int {
	if c.FireAndForget() || c.MaxAttempts < 1 {
		return 1
	}
	return c.MaxAttempts
}

// RetryDelay is the wait after the failed attempt (1 for the first).
func (c HTTPRequestConfig) RetryDelay(attempt int) time.Duration {
	delay := DefaultRetryDelay
	if c.RetryDelaySeconds > 0 {
		delay = time.Duration(c.RetryDelaySeconds) * time.Second
	}
	if c.Backoff == BackoffExponential {
		for range attempt - 1 {
			if delay *= 2; delay >= MaxRetryDelay {
				return MaxRetryDelay
			}
		}
	}
	return delay
}

// Succeeded tells whether a response status counts as success.
func (c HTTPRequestConfig) Succeeded(status int) bool {
	if len(c.SuccessStatuses) == 0 {
		return status >= 200 && status < 300
	}
	return slices.Contains(c.SuccessStatuses, status)
}

// Retryable tells whether a failed response is worth trying again.
func Retryable(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

// SealSecrets seals the values of the secret headers of the http_request
// nodes and drops what non-secret headers had sealed. Configs that do not
// decode are left alone: the draft reports them.
func (g Graph) SealSecrets(seal func([]byte) ([]byte, error)) (Graph, error) {
	out := g
	out.Nodes = slices.Clone(g.Nodes)
	for i, n := range out.Nodes {
		var c HTTPRequestConfig
		if n.Type != NodeHTTPRequest || json.Unmarshal(n.Config, &c) != nil {
			continue
		}
		for j, h := range c.Headers {
			switch {
			case !h.Secret:
				c.Headers[j].Sealed = nil
			case h.Value != "":
				sealed, err := seal([]byte(h.Value))
				if err != nil {
					return g, err
				}
				c.Headers[j].Value, c.Headers[j].Sealed = "", sealed
			}
		}
		out.Nodes[i].Config, _ = json.Marshal(c) // decoded above: marshals
	}
	return out, nil
}

func httpRequestProblems(c HTTPRequestConfig) []string {
	var out []string
	if !slices.Contains(httpMethods, c.Method) {
		out = append(out, "escolha o método HTTP")
	}
	switch u := strings.TrimSpace(c.URL); {
	case u == "":
		out = append(out, "informe a URL")
	case !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://"):
		out = append(out, "a URL precisa começar com http:// ou https://")
	}
	for _, h := range c.Headers {
		switch {
		case !validHeaderName(h.Name):
			out = append(out, fmt.Sprintf("nome de header inválido: %q", h.Name))
		case h.Secret && h.Value == "" && len(h.Sealed) == 0:
			out = append(out, fmt.Sprintf("informe o valor do header %s", h.Name))
		}
	}
	for _, text := range append([]string{c.URL, c.Body}, headerValues(c.Headers)...) {
		if err := checkPlaceholders(text); err != nil {
			out = append(out, err.Error())
		}
	}
	return append(out, httpOptionProblems(c)...)
}

func httpOptionProblems(c HTTPRequestConfig) []string {
	var out []string
	if c.Mode != "" && c.Mode != HTTPModeWait && c.Mode != HTTPModeFireAndForget {
		out = append(out, fmt.Sprintf("modo desconhecido: %q", c.Mode))
	}
	if c.TimeoutSeconds < 0 || c.TimeoutSeconds > httpTimeoutSecondsMax {
		out = append(out, fmt.Sprintf("o timeout vai de 1 a %d segundos", httpTimeoutSecondsMax))
	}
	for _, s := range c.SuccessStatuses {
		if s < 100 || s > 599 {
			out = append(out, fmt.Sprintf("status de sucesso inválido: %d", s))
			break
		}
	}
	if c.MaxAttempts < 0 || c.MaxAttempts > MaxHTTPAttempts {
		out = append(out, fmt.Sprintf("as tentativas vão de 1 a %d", MaxHTTPAttempts))
	}
	if c.RetryDelaySeconds < 0 || c.RetryDelaySeconds > retryDelaySecondsMax {
		out = append(out, fmt.Sprintf("o intervalo entre tentativas vai de 1 a %d segundos", retryDelaySecondsMax))
	}
	if c.Backoff != "" && c.Backoff != BackoffFixed && c.Backoff != BackoffExponential {
		out = append(out, fmt.Sprintf("backoff desconhecido: %q", c.Backoff))
	}
	return out
}

func headerValues(headers []HTTPHeader) []string {
	out := make([]string, len(headers))
	for i, h := range headers {
		out[i] = h.Value
	}
	return out
}

// validHeaderName accepts the token characters of RFC 9110.
func validHeaderName(name string) bool {
	return name != "" && !strings.ContainsFunc(name, func(r rune) bool {
		return r > 0x7e || r <= ' ' || strings.ContainsRune(`"(),/:;<=>?@[\]{}`, r)
	})
}

var placeholder = regexp.MustCompile(`\{\{\s*([^{}\s]*)\s*\}\}`)

// cardFields are the card fields placeholders read, by name.
var cardFields = map[string]func(Card) string{
	"id":             func(c Card) string { return c.ID.String() },
	"title":          func(c Card) string { return c.Title },
	"description":    func(c Card) string { return c.Description },
	"lane_id":        func(c Card) string { return c.LaneID.String() },
	"card_type_id":   func(c Card) string { return c.CardTypeID.String() },
	"project_id":     func(c Card) string { return optionalID(c.ProjectID) },
	"parent_card_id": func(c Card) string { return optionalID(c.ParentCardID) },
}

func optionalID(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// checkPlaceholders reports the first placeholder that names nothing.
func checkPlaceholders(text string) error {
	for _, m := range placeholder.FindAllStringSubmatch(text, -1) {
		if !knownPlaceholder(m[1]) {
			return fmt.Errorf("placeholder desconhecido: %s (use card.<campo>, card.properties.<chave>, input ou input.<caminho>)", m[0])
		}
	}
	return nil
}

func knownPlaceholder(path string) bool {
	root, rest, _ := strings.Cut(path, ".")
	switch root {
	case "input":
		return true
	case "card":
		if key, ok := strings.CutPrefix(rest, "properties."); ok {
			return key != ""
		}
		_, ok := cardFields[rest]
		return ok
	}
	return false
}

// RenderTemplate replaces the placeholders of text:
//   - {{card.<field>}}: id, title, description, lane_id, card_type_id,
//     project_id or parent_card_id of the card;
//   - {{card.properties.<key>}}: a property of the card;
//   - {{input}}: what the node before handed on (see MessageFrom);
//   - {{input.<path>}}: a value in it, by keys and array indexes split by
//     dots, inside its structured output when it has one.
//
// escape encodes each value for where it goes; values that are not
// strings come as JSON, with text false. Unknown placeholders and missing
// values become empty strings.
func RenderTemplate(text string, card Card, input json.RawMessage, escape func(value string, text bool) string) string {
	return placeholder.ReplaceAllStringFunc(text, func(m string) string {
		return escape(lookup(placeholder.FindStringSubmatch(m)[1], card, input))
	})
}

func lookup(path string, card Card, input json.RawMessage) (string, bool) {
	root, rest, _ := strings.Cut(path, ".")
	switch {
	case path == "input":
		return MessageFrom(input), true
	case root == "input":
		return inputValue(input, strings.Split(rest, "."))
	case root != "card":
		return "", true
	}
	if key, ok := strings.CutPrefix(rest, "properties."); ok {
		return card.Properties[key], true
	}
	if field, ok := cardFields[rest]; ok {
		return field(card), true
	}
	return "", true
}

// inputValue walks path in the input, inside its output when it has one.
func inputValue(input json.RawMessage, path []string) (string, bool) {
	var v any
	if json.Unmarshal(input, &v) != nil {
		return "", true
	}
	if m, ok := v.(map[string]any); ok && m["output"] != nil {
		v = m["output"]
	}
	for _, key := range path {
		switch node := v.(type) {
		case map[string]any:
			v = node[key]
		case []any:
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i >= len(node) {
				return "", true
			}
			v = node[i]
		default:
			return "", true
		}
	}
	switch value := v.(type) {
	case nil:
		return "", true
	case string:
		return value, true
	default:
		b, _ := json.Marshal(value) // decoded from JSON: marshals
		return string(b), false
	}
}

// EscapeURL encodes a value for a URL path or query.
func EscapeURL(s string, _ bool) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// EscapeJSON encodes a value for a JSON body: a string's characters are
// escaped, without the quotes, so "{{card.title}}" stays a valid string;
// JSON values (objects, numbers) go in as they are.
func EscapeJSON(s string, text bool) string {
	if !text {
		return s
	}
	b, _ := json.Marshal(s) // strings always marshal
	return string(b[1 : len(b)-1])
}

// EscapeHeader keeps a value on one header line.
func EscapeHeader(s string, _ bool) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}
