package specdata

import (
	"context"
	_ "embed"
	"fmt"

	openapi "github.com/pierskarsenbarg/pulumi-openapi-provider"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
)

// AdaptedSpec is the REST-shaped OpenAPI document produced by `make generate`.
//
//go:embed openapi.adapted.yaml
var AdaptedSpec []byte

// PathMap is the JSON list of specadapt.Rule values produced by `make generate`.
//
//go:embed pathmap.json
var PathMap []byte

// Overrides maps discovered OpenAPI resource names onto Pulumi tokens and ID fields.
func Overrides() map[string]openapi.ResourceOverride {
	return map[string]openapi.ResourceOverride{
		"*":          {Diff: ignoreOutputOnlyDiff},
		"Volumes":    {Token: "podman:volumes:Volume", IDField: "Name"},
		"Containers": {Token: "podman:containers:Container", IDField: "Id", Diff: replaceOnInputDiff},
		"Pods":       {Token: "podman:pods:Pod", IDField: "Id", Diff: replaceOnInputDiff},
		"Networks":   {Token: "podman:networks:Network", IDField: "id"},
		"Secrets":    {Token: "podman:secrets:Secret", IDField: "ID"},
		"Images":     {Token: "podman:images:Image", IDField: "id"},
		"Artifacts":  {Token: "podman:artifacts:Artifact"},
		"Quadlets":   {Skip: true},
	}
}

// ignoreOutputOnlyDiff compares program inputs to state and ignores extra inspect
// fields that Podman adds on GET. Those would otherwise look like property deletes.
func ignoreOutputOnlyDiff(_ context.Context, req p.DiffRequest) (p.DiffResponse, error) {
	detailed := map[string]p.PropertyDiff{}
	req.Inputs.All(func(key string, newVal property.Value) bool {
		oldVal, hasOld := req.State.GetOk(key)
		if !hasOld || fmt.Sprintf("%v", oldVal) != fmt.Sprintf("%v", newVal) {
			detailed[key] = p.PropertyDiff{Kind: p.Update}
		}
		return true
	})
	return p.DiffResponse{HasChanges: len(detailed) > 0, DetailedDiff: detailed}, nil
}

func replaceOnInputDiff(ctx context.Context, req p.DiffRequest) (p.DiffResponse, error) {
	resp, err := ignoreOutputOnlyDiff(ctx, req)
	if err != nil {
		return resp, err
	}
	for k, d := range resp.DetailedDiff {
		d.Kind = p.UpdateReplace
		resp.DetailedDiff[k] = d
	}
	resp.DeleteBeforeReplace = true
	return resp, nil
}
