// Package transport talks to a Podman REST socket and rewrites the REST-shaped
// paths produced by specadapt back to upstream Libpod URLs.
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/geoffsee/pulumi-podman/internal/specadapt"
)

// Conn is a parsed Podman connection URI.
type Conn struct {
	Network string // unix or tcp
	Address string
}

// ParseConn parses CONTAINER_HOST-style URIs.
func ParseConn(raw string) (Conn, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Conn{}, fmt.Errorf("empty connection URI")
	}
	if strings.HasPrefix(raw, "unix://") {
		return Conn{Network: "unix", Address: strings.TrimPrefix(raw, "unix://")}, nil
	}
	if strings.HasPrefix(raw, "unix:") {
		return Conn{Network: "unix", Address: strings.TrimPrefix(raw, "unix:")}, nil
	}
	if strings.HasPrefix(raw, "/") {
		return Conn{Network: "unix", Address: raw}, nil
	}
	if strings.HasPrefix(raw, "tcp://") {
		return Conn{Network: "tcp", Address: strings.TrimPrefix(raw, "tcp://")}, nil
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return Conn{}, err
		}
		return Conn{Network: "tcp", Address: u.Host}, nil
	}
	if strings.HasPrefix(raw, "ssh://") {
		return Conn{}, fmt.Errorf("ssh connections use podman:connectionUri with sshIdentityFile or sshPrivateKey, or set CONTAINER_HOST=ssh://... with CONTAINER_SSHKEY")
	}
	return Conn{}, fmt.Errorf("unsupported connection URI %q", raw)
}

// DetectConn picks a local Podman API endpoint from the environment or common socket paths.
// ssh:// URIs are not opened here; use Endpoint.Apply for SSH tunnels.
func DetectConn() (Conn, error) {
	for _, env := range []string{"CONTAINER_HOST", "PODMAN_HOST"} {
		if v := os.Getenv(env); v != "" {
			return ParseConn(v)
		}
	}
	return probeLocalSockets()
}

// RoundTripper rewrites REST paths and dials the Podman socket.
type RoundTripper struct {
	endpoint *Endpoint
	rules    []compiledRule
	base     http.RoundTripper
}

type compiledRule struct {
	specadapt.Rule
	re *regexp.Regexp
}

var versionPrefix = regexp.MustCompile(`^/v\d+\.\d+\.\d+`)

// RewriteAPIPrefix replaces or inserts the /vX.0.0 prefix on upstream paths.
func RewriteAPIPrefix(rules []specadapt.Rule, version string) []specadapt.Rule {
	prefix := specadapt.APIPrefix(version)
	out := make([]specadapt.Rule, len(rules))
	for i, r := range rules {
		r.UpstreamPath = rewritePrefix(r.UpstreamPath, prefix)
		r.StartPath = rewritePrefix(r.StartPath, prefix)
		out[i] = r
	}
	return out
}

func rewritePrefix(path, prefix string) string {
	if path == "" {
		return path
	}
	if versionPrefix.MatchString(path) {
		return versionPrefix.ReplaceAllString(path, prefix)
	}
	if strings.HasPrefix(path, "/libpod/") || path == "/libpod" {
		return prefix + path
	}
	return path
}

// New returns an HTTP client that targets Podman through ep.
func New(ep *Endpoint, rules []specadapt.Rule) *http.Client {
	if v := os.Getenv("PODMAN_API_VERSION"); v != "" {
		rules = RewriteAPIPrefix(rules, v)
	}
	compiled := make([]compiledRule, 0, len(rules))
	for _, r := range rules {
		compiled = append(compiled, compiledRule{Rule: r, re: pathRegexp(r.AdaptedPath)})
	}
	return &http.Client{
		Timeout: 15 * time.Minute,
		Transport: &RoundTripper{
			endpoint: ep,
			rules:    compiled,
			base: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return ep.Dial(ctx)
				},
				DisableCompression: true,
			},
		},
	}
}

func pathRegexp(tmpl string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(tmpl); {
		if tmpl[i] == '{' {
			end := strings.IndexByte(tmpl[i:], '}')
			if end < 0 {
				break
			}
			// `.+` (not `[^/]+`) so image/artifact names with slashes match,
			// e.g. GET /images/localhost/busybox:latest.
			b.WriteString("(?P<" + tmpl[i+1:i+end] + ">.+)")
			i += end + 1
			continue
		}
		b.WriteString(regexp.QuoteMeta(string(tmpl[i])))
		i++
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func (t *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rule, params := t.match(req.Method, req.URL.Path)
	if rule != nil {
		if err := ApplyRule(req, *rule, params); err != nil {
			return nil, err
		}
	}
	req.URL.Scheme = "http"
	req.URL.Host = "d"
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if rule != nil && rule.StartPath != "" && req.Method == "POST" && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := t.startAfterCreate(req.Context(), rule.StartPath, resp); err != nil {
			_ = resp.Body.Close()
			return nil, err
		}
	}
	return normalizeResponse(resp)
}

// coerceJSONObject rewrites a JSON array body to {}. The OpenAPI runtime always
// unmarshals success bodies into map[string]any, but Podman delete endpoints
// return arrays of reports.
func coerceJSONObject(raw []byte) []byte {
	trim := bytes.TrimSpace(raw)
	if len(trim) > 0 && trim[0] == '[' {
		return []byte("{}")
	}
	return raw
}

func normalizeResponse(resp *http.Response) (*http.Response, error) {
	if resp.Body == nil {
		return resp, nil
	}
	raw, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	raw = coerceJSONObject(raw)
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	resp.ContentLength = int64(len(raw))
	return resp, nil
}

func (t *RoundTripper) match(method, path string) (*specadapt.Rule, map[string]string) {
	for i := range t.rules {
		r := &t.rules[i]
		if r.Method != method {
			continue
		}
		m := r.re.FindStringSubmatch(path)
		if m == nil {
			continue
		}
		params := map[string]string{}
		for i, name := range r.re.SubexpNames() {
			if i == 0 || name == "" {
				continue
			}
			params[name] = m[i]
		}
		return &r.Rule, params
	}
	return nil, nil
}

// ApplyRule rewrites req to the upstream Podman operation described by rule.
func ApplyRule(req *http.Request, rule specadapt.Rule, params map[string]string) error {
	up := rule.UpstreamPath
	id := params["id"]
	if id == "" {
		id = params["name"]
	}
	up = strings.ReplaceAll(up, "{id}", id)
	up = strings.ReplaceAll(up, "{name}", id)
	for k, v := range params {
		up = strings.ReplaceAll(up, "{"+k+"}", v)
	}

	q := req.URL.Query()
	for k, v := range rule.DefaultQuery {
		if q.Get(k) == "" {
			q.Set(k, v)
		}
	}

	var body map[string]any
	if req.Body != nil && req.Body != http.NoBody {
		raw, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return err
		}
		if len(raw) > 0 && (rule.BodyMode == "query" || rule.BodyMode == "rawString" || len(rule.QueryFromBody) > 0 || strings.HasSuffix(up, "/libpod/containers/create")) {
			_ = json.Unmarshal(raw, &body)
		} else {
			req.Body = io.NopCloser(bytes.NewReader(raw))
			req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }
			req.ContentLength = int64(len(raw))
		}
	}

	if body != nil {
		// The OpenAPI runtime translates only top-level input aliases. Podman
		// expects snake_case keys inside the generated PortMapping objects too.
		if strings.HasSuffix(up, "/libpod/containers/create") {
			if ports, ok := body["portmappings"].([]any); ok {
				for _, port := range ports {
					if mapping, ok := port.(map[string]any); ok {
						for sdk, api := range map[string]string{
							"containerPort": "container_port", "hostPort": "host_port", "hostIp": "host_ip",
						} {
							if value, exists := mapping[sdk]; exists {
								mapping[api] = value
								delete(mapping, sdk)
							}
						}
					}
				}
			}
		}
		for _, name := range rule.QueryFromBody {
			if v, ok := body[name]; ok {
				q.Set(name, fmt.Sprint(v))
				delete(body, name)
			}
		}
		switch rule.BodyMode {
		case "query":
			req.Body = http.NoBody
			req.ContentLength = 0
			req.Header.Del("Content-Type")
		case "rawString":
			field := rule.RawStringField
			if field == "" {
				field = "data"
			}
			payload := fmt.Sprint(body[field])
			req.Body = io.NopCloser(strings.NewReader(payload))
			req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(payload)), nil }
			req.ContentLength = int64(len(payload))
			req.Header.Set("Content-Type", "application/octet-stream")
		default:
			raw, err := json.Marshal(body)
			if err != nil {
				return err
			}
			req.Body = io.NopCloser(bytes.NewReader(raw))
			req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }
			req.ContentLength = int64(len(raw))
		}
	}

	req.URL.Path = up
	req.URL.RawQuery = q.Encode()
	req.Method = rule.UpstreamMethod
	return nil
}

func (t *RoundTripper) startAfterCreate(ctx context.Context, startPath string, createResp *http.Response) error {
	raw, err := io.ReadAll(createResp.Body)
	_ = createResp.Body.Close()
	if err != nil {
		return err
	}
	createResp.Body = io.NopCloser(bytes.NewReader(raw))
	id := extractCreateID(raw)
	if id == "" {
		return nil
	}
	path := strings.ReplaceAll(startPath, "{id}", id)
	path = strings.ReplaceAll(path, "{name}", id)
	startReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://d"+path, nil)
	if err != nil {
		return err
	}
	startResp, err := t.base.RoundTrip(startReq)
	if err != nil {
		return fmt.Errorf("start resource: %w", err)
	}
	defer startResp.Body.Close()
	if startResp.StatusCode >= 300 {
		msg, _ := io.ReadAll(startResp.Body)
		return fmt.Errorf("start resource: HTTP %d: %s", startResp.StatusCode, msg)
	}
	return nil
}

func extractCreateID(raw []byte) string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	for _, k := range []string{"Id", "ID", "id", "Name", "name"} {
		if v, ok := m[k]; ok {
			s := fmt.Sprint(v)
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}
