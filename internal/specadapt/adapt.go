// Package specadapt rewrites the Podman Libpod Swagger 2.0 document into a
// REST-shaped OpenAPI spec that pulumi-openapi-provider can discover, and a
// path map that sends runtime HTTP calls back to the real Podman endpoints.
package specadapt

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Rule maps one adapted (REST) operation onto the upstream Podman path.
type Rule struct {
	Method         string            `json:"method"`
	AdaptedPath    string            `json:"adaptedPath"`
	UpstreamMethod string            `json:"upstreamMethod"`
	UpstreamPath   string            `json:"upstreamPath"`
	QueryFromBody  []string          `json:"queryFromBody,omitempty"`
	DefaultQuery   map[string]string `json:"defaultQuery,omitempty"`
	BodyMode       string            `json:"bodyMode,omitempty"` // json (default), query, rawString
	RawStringField string            `json:"rawStringField,omitempty"`
	StartPath      string            `json:"startPath,omitempty"`
}

// Result is the adapted spec plus the runtime path map.
type Result struct {
	Spec    []byte
	PathMap []byte
	Rules   []Rule
}

// Adapt rewrites a Podman swagger 2.0 document.
func Adapt(swaggerYAML []byte) (Result, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(swaggerYAML, &doc); err != nil {
		return Result{}, fmt.Errorf("parse swagger: %w", err)
	}

	rawPaths, _ := doc["paths"].(map[string]any)
	if rawPaths == nil {
		return Result{}, fmt.Errorf("swagger has no paths")
	}

	// Only the Libpod API. Compat endpoints duplicate the same resources.
	libpod := map[string]any{}
	for p, item := range rawPaths {
		if strings.HasPrefix(p, "/libpod/") {
			libpod[p] = item
		}
	}

	rules := []Rule{}
	paths := cloneMap(libpod)

	moveInspectJSON(paths, &rules)
	moveCreateSuffix(paths, &rules, "create")
	moveCreateSuffix(paths, &rules, "pull")
	moveCreateSuffix(paths, &rules, "add")
	moveUpdateSuffix(paths, &rules)
	synthesizeQueryBodies(paths, &rules)
	keepCRUDOnly(paths)
	dropOrphanItems(paths)
	addIdentityRules(paths, &rules)
	stripLibpodPrefix(paths, &rules)
	renameNameParam(paths, &rules)
	pruneDeadRules(paths, &rules)
	applyPodmanDefaults(&rules)
	applyAPIPrefix(doc, &rules)

	doc["paths"] = paths
	delete(doc, "host")
	doc["schemes"] = []any{"http"}
	if info, ok := doc["info"].(map[string]any); ok {
		info["title"] = "Podman Libpod API"
		info["description"] = "REST-shaped view of the Podman Libpod API, generated for the Pulumi provider. Upstream: https://docs.podman.io/en/latest/_static/api.html"
	}

	specBytes, err := yaml.Marshal(doc)
	if err != nil {
		return Result{}, fmt.Errorf("marshal adapted spec: %w", err)
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].AdaptedPath == rules[j].AdaptedPath {
			return rules[i].Method < rules[j].Method
		}
		return rules[i].AdaptedPath < rules[j].AdaptedPath
	})
	mapBytes, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("marshal path map: %w", err)
	}
	return Result{Spec: specBytes, PathMap: append(mapBytes, '\n'), Rules: rules}, nil
}

func moveInspectJSON(paths map[string]any, rules *[]Rule) {
	for p, item := range cloneMap(paths) {
		if !strings.HasSuffix(p, "/json") {
			continue
		}
		parent := strings.TrimSuffix(p, "/json")
		if !isItemPath(parent) {
			continue
		}
		src, _ := item.(map[string]any)
		get, ok := src["get"].(map[string]any)
		if !ok {
			continue
		}
		dst := pathItem(paths, parent)
		dst["get"] = clone(get)
		delete(paths, p)
		*rules = append(*rules, Rule{
			Method:         "GET",
			AdaptedPath:    parent,
			UpstreamMethod: "GET",
			UpstreamPath:   p,
		})
	}
}

func moveCreateSuffix(paths map[string]any, rules *[]Rule, suffix string) {
	for p, item := range cloneMap(paths) {
		if !strings.HasSuffix(p, "/"+suffix) {
			continue
		}
		parent := strings.TrimSuffix(p, "/"+suffix)
		if isItemPath(parent) {
			continue
		}
		src, _ := item.(map[string]any)
		post, ok := src["post"].(map[string]any)
		if !ok {
			continue
		}
		dst := pathItem(paths, parent)
		if _, exists := dst["post"]; exists && suffix != "create" {
			continue
		}
		dst["post"] = clone(post)
		delete(paths, p)
		rule := Rule{
			Method:         "POST",
			AdaptedPath:    parent,
			UpstreamMethod: "POST",
			UpstreamPath:   p,
		}
		fillBodyMode(post, &rule)
		*rules = append(*rules, rule)
	}
}

func moveUpdateSuffix(paths map[string]any, rules *[]Rule) {
	for p, item := range cloneMap(paths) {
		if !strings.HasSuffix(p, "/update") || !isItemPath(strings.TrimSuffix(p, "/update")) {
			continue
		}
		parent := strings.TrimSuffix(p, "/update")
		// Container /update takes ContainerUpdateOptions (CPU, memory), not SpecGenerator.
		if strings.Contains(parent, "/containers/") {
			continue
		}
		src, _ := item.(map[string]any)
		post, ok := src["post"].(map[string]any)
		if !ok {
			continue
		}
		dst := pathItem(paths, parent)
		dst["patch"] = clone(post)
		delete(paths, p)
		*rules = append(*rules, Rule{
			Method:         "PATCH",
			AdaptedPath:    parent,
			UpstreamMethod: "POST",
			UpstreamPath:   p,
		})
	}
}

func synthesizeQueryBodies(paths map[string]any, rules *[]Rule) {
	for p, item := range paths {
		pi, _ := item.(map[string]any)
		post, ok := pi["post"].(map[string]any)
		if !ok {
			continue
		}
		params, _ := post["parameters"].([]any)
		query := []map[string]any{}
		var bodyParam map[string]any
		var rest []any
		for _, raw := range params {
			pm, _ := raw.(map[string]any)
			switch fmt.Sprint(pm["in"]) {
			case "query":
				query = append(query, pm)
			case "body":
				bodyParam = pm
			default:
				rest = append(rest, raw)
			}
		}
		schema, _ := bodySchema(bodyParam)
		needsSynth := len(query) > 0 && (schema == nil || isStringSchema(schema))
		if !needsSynth {
			continue
		}

		props := map[string]any{}
		required := []any{}
		queryNames := []string{}
		for _, q := range query {
			name := fmt.Sprint(q["name"])
			queryNames = append(queryNames, name)
			prop := map[string]any{"type": "string"}
			if t := q["type"]; t != nil {
				prop["type"] = t
			}
			if d := q["description"]; d != nil {
				prop["description"] = d
			}
			props[name] = prop
			if q["required"] == true {
				required = append(required, name)
			}
		}
		mode := "query"
		rawField := ""
		if isStringSchema(schema) {
			props["data"] = map[string]any{
				"type":        "string",
				"description": "Raw request body payload.",
			}
			required = append(required, "data")
			mode = "rawString"
			rawField = "data"
		}
		newBody := map[string]any{
			"in":       "body",
			"name":     "create",
			"required": true,
			"schema": map[string]any{
				"type":       "object",
				"properties": props,
				"required":   required,
			},
		}
		post["parameters"] = append(rest, newBody)
		for i := range *rules {
			r := &(*rules)[i]
			if r.Method == "POST" && r.AdaptedPath == p {
				r.QueryFromBody = queryNames
				r.BodyMode = mode
				r.RawStringField = rawField
			}
		}
	}
}

func keepCRUDOnly(paths map[string]any) {
	itemParents := map[string]bool{}
	for p := range paths {
		if isItemPath(p) {
			itemParents[collectionPath(p)] = true
		}
	}
	for p := range cloneMap(paths) {
		if isItemPath(p) {
			pi, _ := paths[p].(map[string]any)
			keep := map[string]any{}
			for _, m := range []string{"get", "put", "patch", "delete"} {
				if op, ok := pi[m]; ok {
					keep[m] = op
				}
			}
			if len(keep) == 0 {
				delete(paths, p)
				continue
			}
			paths[p] = keep
			continue
		}
		if !itemParents[p] {
			delete(paths, p)
			continue
		}
		pi, _ := paths[p].(map[string]any)
		keep := map[string]any{}
		if op, ok := pi["post"]; ok {
			keep["post"] = op
		}
		if len(keep) == 0 {
			delete(paths, p)
			continue
		}
		paths[p] = keep
	}
}

// dropOrphanItems removes item paths whose collection has no POST create.
// Those cannot become Pulumi resources (create is mandatory).
func dropOrphanItems(paths map[string]any) {
	for p := range cloneMap(paths) {
		if !isItemPath(p) {
			continue
		}
		col := collectionPath(p)
		pi, _ := paths[col].(map[string]any)
		if _, ok := pi["post"]; !ok {
			delete(paths, p)
		}
	}
}

// pruneDeadRules drops path-map entries whose adapted operation is no longer in the spec.
func pruneDeadRules(paths map[string]any, rules *[]Rule) {
	live := map[string]bool{}
	for p, item := range paths {
		pi, _ := item.(map[string]any)
		for method, key := range map[string]string{
			"GET": "get", "POST": "post", "PUT": "put", "PATCH": "patch", "DELETE": "delete",
		} {
			if _, ok := pi[key]; ok {
				live[method+" "+p] = true
			}
		}
	}
	out := make([]Rule, 0, len(*rules))
	for _, r := range *rules {
		if live[r.Method+" "+r.AdaptedPath] {
			out = append(out, r)
		}
	}
	*rules = out
}

func addIdentityRules(paths map[string]any, rules *[]Rule) {
	covered := map[string]bool{}
	for _, r := range *rules {
		covered[r.Method+" "+r.AdaptedPath] = true
	}
	for p, item := range paths {
		pi, _ := item.(map[string]any)
		for method, key := range map[string]string{
			"GET": "get", "POST": "post", "PUT": "put", "PATCH": "patch", "DELETE": "delete",
		} {
			if _, ok := pi[key]; !ok {
				continue
			}
			if covered[method+" "+p] {
				continue
			}
			*rules = append(*rules, Rule{
				Method:         method,
				AdaptedPath:    p,
				UpstreamMethod: method,
				UpstreamPath:   p,
			})
		}
	}
}

func stripLibpodPrefix(paths map[string]any, rules *[]Rule) {
	next := map[string]any{}
	for p, item := range paths {
		np := strings.TrimPrefix(p, "/libpod")
		if np == "" {
			np = "/"
		}
		next[np] = item
	}
	for k := range paths {
		delete(paths, k)
	}
	for k, v := range next {
		paths[k] = v
	}
	for i := range *rules {
		r := &(*rules)[i]
		r.AdaptedPath = strings.TrimPrefix(r.AdaptedPath, "/libpod")
		if r.AdaptedPath == "" {
			r.AdaptedPath = "/"
		}
	}
}

func renameNameParam(paths map[string]any, rules *[]Rule) {
	next := map[string]any{}
	for p, item := range paths {
		np := strings.ReplaceAll(p, "{name}", "{id}")
		pi, _ := clone(item).(map[string]any)
		for _, key := range []string{"get", "post", "put", "patch", "delete"} {
			op, ok := pi[key].(map[string]any)
			if !ok {
				continue
			}
			params, _ := op["parameters"].([]any)
			for i, raw := range params {
				pm, _ := clone(raw).(map[string]any)
				if fmt.Sprint(pm["in"]) == "path" && fmt.Sprint(pm["name"]) == "name" {
					pm["name"] = "id"
				}
				params[i] = pm
			}
			if params != nil {
				op["parameters"] = params
			}
			pi[key] = op
		}
		next[np] = pi
	}
	for k := range paths {
		delete(paths, k)
	}
	for k, v := range next {
		paths[k] = v
	}
	for i := range *rules {
		(*rules)[i].AdaptedPath = strings.ReplaceAll((*rules)[i].AdaptedPath, "{name}", "{id}")
	}
}

func applyPodmanDefaults(rules *[]Rule) {
	for i := range *rules {
		r := &(*rules)[i]
		switch {
		case r.AdaptedPath == "/containers" && r.Method == "POST":
			r.StartPath = "/libpod/containers/{id}/start"
		case r.AdaptedPath == "/pods" && r.Method == "POST":
			r.StartPath = "/libpod/pods/{id}/start"
		case strings.HasPrefix(r.AdaptedPath, "/containers/{id}") && r.Method == "DELETE":
			r.DefaultQuery = map[string]string{"force": "true", "v": "true"}
		case strings.HasPrefix(r.AdaptedPath, "/pods/{id}") && r.Method == "DELETE":
			r.DefaultQuery = map[string]string{"force": "true"}
		case r.AdaptedPath == "/images" && r.Method == "POST":
			if r.DefaultQuery == nil {
				r.DefaultQuery = map[string]string{}
			}
			r.DefaultQuery["quiet"] = "true"
		}
	}
}

func applyAPIPrefix(doc map[string]any, rules *[]Rule) {
	prefix := apiPrefixFromInfo(doc)
	for i := range *rules {
		r := &(*rules)[i]
		r.UpstreamPath = prefixPath(r.UpstreamPath, prefix)
		if r.StartPath != "" {
			r.StartPath = prefixPath(r.StartPath, prefix)
		}
	}
}

func apiPrefixFromInfo(doc map[string]any) string {
	info, _ := doc["info"].(map[string]any)
	return APIPrefix(fmt.Sprint(info["version"]))
}

// APIPrefix turns a Podman swagger version (e.g. "6.0.0") into the URI prefix
// the engine actually serves ("/v6.0.0"). Unversioned /libpod/* paths 404.
func APIPrefix(version string) string {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if version == "" || version == "<nil>" {
		return "/v5.0.0"
	}
	major := version
	if i := strings.IndexByte(version, '.'); i >= 0 {
		major = version[:i]
	}
	if major == "" {
		return "/v5.0.0"
	}
	return "/v" + major + ".0.0"
}

func prefixPath(path, prefix string) string {
	if path == "" {
		return path
	}
	if strings.HasPrefix(path, "/v") {
		return path
	}
	return prefix + path
}

func fillBodyMode(post map[string]any, rule *Rule) {
	params, _ := post["parameters"].([]any)
	var queryNames []string
	var body map[string]any
	for _, raw := range params {
		pm, _ := raw.(map[string]any)
		switch fmt.Sprint(pm["in"]) {
		case "query":
			queryNames = append(queryNames, fmt.Sprint(pm["name"]))
		case "body":
			body = pm
		}
	}
	schema, _ := bodySchema(body)
	if schema == nil && len(queryNames) > 0 {
		rule.BodyMode = "query"
		rule.QueryFromBody = queryNames
		return
	}
	if isStringSchema(schema) {
		rule.BodyMode = "rawString"
		rule.RawStringField = "data"
		rule.QueryFromBody = queryNames
	}
}

func bodySchema(body map[string]any) (map[string]any, bool) {
	if body == nil {
		return nil, false
	}
	schema, _ := body["schema"].(map[string]any)
	return schema, schema != nil
}

func isStringSchema(schema map[string]any) bool {
	if schema == nil {
		return false
	}
	if _, isRef := schema["$ref"]; isRef {
		return false
	}
	return fmt.Sprint(schema["type"]) == "string"
}

func isItemPath(p string) bool {
	segs := splitPath(p)
	if len(segs) == 0 {
		return false
	}
	last := segs[len(segs)-1]
	return strings.HasPrefix(last, "{") && strings.HasSuffix(last, "}")
}

func collectionPath(item string) string {
	segs := splitPath(item)
	if len(segs) == 0 {
		return "/"
	}
	return "/" + strings.Join(segs[:len(segs)-1], "/")
}

func splitPath(p string) []string {
	var segs []string
	for _, s := range strings.Split(strings.Trim(p, "/"), "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	return segs
}

func pathItem(paths map[string]any, p string) map[string]any {
	if existing, ok := paths[p].(map[string]any); ok {
		return existing
	}
	n := map[string]any{}
	paths[p] = n
	return n
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func clone(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return v
	}
	return out
}
