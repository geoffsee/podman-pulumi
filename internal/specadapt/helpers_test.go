package specadapt

import "testing"

func TestPrefixPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path, prefix, want string
	}{
		{"", "/v6.0.0", ""},
		{"/libpod/volumes/create", "/v6.0.0", "/v6.0.0/libpod/volumes/create"},
		{"/v6.0.0/libpod/volumes/create", "/v5.0.0", "/v6.0.0/libpod/volumes/create"},
		{"/libpod", "/v6.0.0", "/v6.0.0/libpod"},
	}
	for _, tc := range cases {
		if got := prefixPath(tc.path, tc.prefix); got != tc.want {
			t.Errorf("prefixPath(%q, %q) = %q, want %q", tc.path, tc.prefix, got, tc.want)
		}
	}
}

func TestItemAndCollectionPath(t *testing.T) {
	t.Parallel()
	if isItemPath("/volumes") {
		t.Fatal("collection is not an item")
	}
	if isItemPath("") || isItemPath("/") {
		t.Fatal("empty path is not an item")
	}
	if !isItemPath("/volumes/{name}") || !isItemPath("/libpod/images/{id}") {
		t.Fatal("expected item paths")
	}
	if collectionPath("/volumes/{name}") != "/volumes" {
		t.Fatalf("collection = %s", collectionPath("/volumes/{name}"))
	}
	if collectionPath("/") != "/" {
		t.Fatalf("root collection = %s", collectionPath("/"))
	}
	if collectionPath("") != "/" {
		t.Fatalf("empty collection = %s", collectionPath(""))
	}
}

func TestIsStringSchema(t *testing.T) {
	t.Parallel()
	if isStringSchema(nil) {
		t.Fatal("nil")
	}
	if isStringSchema(map[string]any{"$ref": "#/definitions/X"}) {
		t.Fatal("$ref is not a string schema")
	}
	if isStringSchema(map[string]any{"type": "object"}) {
		t.Fatal("object")
	}
	if !isStringSchema(map[string]any{"type": "string"}) {
		t.Fatal("string")
	}
}

func TestSplitPath(t *testing.T) {
	t.Parallel()
	if got := splitPath("/a//b/"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got %v", got)
	}
	if got := splitPath("/"); len(got) != 0 {
		t.Fatalf("root = %v", got)
	}
}
