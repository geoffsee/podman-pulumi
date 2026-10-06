package specdata

import (
	"context"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
)

func TestIgnoreOutputOnlyDiff(t *testing.T) {
	req := p.DiffRequest{
		Inputs: property.NewMap(map[string]property.Value{
			"name":  property.New("demo"),
			"image": property.New("localhost/rubix-hello:latest"),
		}),
		State: property.NewMap(map[string]property.Value{
			"name":  property.New("demo"),
			"image": property.New("localhost/rubix-hello:latest"),
			"state": property.New("running"),
			"Id":    property.New("abc"),
		}),
	}
	resp, err := ignoreOutputOnlyDiff(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.HasChanges {
		t.Fatalf("inspect-only fields should not diff: %+v", resp.DetailedDiff)
	}
}

func TestReplaceOnInputDiff(t *testing.T) {
	req := p.DiffRequest{
		Inputs: property.NewMap(map[string]property.Value{
			"image": property.New("other:latest"),
		}),
		State: property.NewMap(map[string]property.Value{
			"image": property.New("localhost/rubix-hello:latest"),
			"state": property.New("running"),
		}),
	}
	resp, err := replaceOnInputDiff(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.HasChanges || !resp.DeleteBeforeReplace {
		t.Fatalf("expected replace: %+v", resp)
	}
	if resp.DetailedDiff["image"].Kind != p.UpdateReplace {
		t.Fatalf("image diff = %+v", resp.DetailedDiff["image"])
	}
}

func TestIgnoreOutputOnlyDiffDetectsInputChange(t *testing.T) {
	req := p.DiffRequest{
		Inputs: property.NewMap(map[string]property.Value{
			"name": property.New("other"),
		}),
		State: property.NewMap(map[string]property.Value{
			"name":  property.New("demo"),
			"state": property.New("running"),
		}),
	}
	resp, err := ignoreOutputOnlyDiff(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.HasChanges {
		t.Fatal("expected a change on name")
	}
	if _, ok := resp.DetailedDiff["name"]; !ok {
		t.Fatalf("detailed = %+v", resp.DetailedDiff)
	}
	if _, ok := resp.DetailedDiff["state"]; ok {
		t.Fatal("inspect-only state should not appear in the diff")
	}
}

func TestReplaceOnInputDiffNoChange(t *testing.T) {
	req := p.DiffRequest{
		Inputs: property.NewMap(map[string]property.Value{
			"image": property.New("pause"),
		}),
		State: property.NewMap(map[string]property.Value{
			"image": property.New("pause"),
			"Id":    property.New("abc"),
		}),
	}
	resp, err := replaceOnInputDiff(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.HasChanges {
		t.Fatalf("inspect-only extras should not replace: %+v", resp)
	}
	if !resp.DeleteBeforeReplace {
		t.Fatal("replace hook still flags delete-before-replace")
	}
}

func TestIgnoreOutputOnlyDiffNewInput(t *testing.T) {
	req := p.DiffRequest{
		Inputs: property.NewMap(map[string]property.Value{
			"name":  property.New("demo"),
			"label": property.New("x"),
		}),
		State: property.NewMap(map[string]property.Value{
			"name": property.New("demo"),
		}),
	}
	resp, err := ignoreOutputOnlyDiff(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.HasChanges {
		t.Fatal("new input key should diff")
	}
	if resp.DetailedDiff["label"].Kind != p.Update {
		t.Fatalf("label = %+v", resp.DetailedDiff["label"])
	}
}

func TestIgnoreOutputOnlyDiffEmptyInputs(t *testing.T) {
	req := p.DiffRequest{
		Inputs: property.NewMap(map[string]property.Value{}),
		State: property.NewMap(map[string]property.Value{
			"Name": property.New("demo"),
		}),
	}
	resp, err := ignoreOutputOnlyDiff(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.HasChanges {
		t.Fatalf("empty inputs against inspect state should not diff: %+v", resp)
	}
}

func TestReplaceOnInputDiffMultipleKeys(t *testing.T) {
	req := p.DiffRequest{
		Inputs: property.NewMap(map[string]property.Value{
			"image":   property.New("other"),
			"command": property.New("sleep"),
		}),
		State: property.NewMap(map[string]property.Value{
			"image":   property.New("pause"),
			"command": property.New("true"),
			"Id":      property.New("abc"),
		}),
	}
	resp, err := replaceOnInputDiff(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.HasChanges || !resp.DeleteBeforeReplace {
		t.Fatalf("expected replace: %+v", resp)
	}
	for _, k := range []string{"image", "command"} {
		if resp.DetailedDiff[k].Kind != p.UpdateReplace {
			t.Fatalf("%s = %+v", k, resp.DetailedDiff[k])
		}
	}
}
