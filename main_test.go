package main

import (
	"errors"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		check   func(t *testing.T, cfg cliConfig)
		wantErr error
	}{
		{
			name: "get namespaced resource",
			args: []string{"get", "widgets.example.com", "sample", "-n", "demo"},
			check: func(t *testing.T, cfg cliConfig) {
				t.Helper()
				if cfg.verb != "get" || cfg.resource != "widgets.example.com" || cfg.name != "sample" || cfg.namespace != "demo" {
					t.Fatalf("unexpected config: %#v", cfg)
				}
			},
		},
		{
			name: "watch selector",
			args: []string{"watch", "widgets", "-l", "app=test"},
			check: func(t *testing.T, cfg cliConfig) {
				t.Helper()
				if !cfg.watch || cfg.selector != "app=test" {
					t.Fatalf("unexpected config: %#v", cfg)
				}
			},
		},
		{
			name: "update with filename",
			args: []string{"update", "-f", "resource.yaml", "--context", "demo"},
			check: func(t *testing.T, cfg cliConfig) {
				t.Helper()
				if cfg.filename != "resource.yaml" {
					t.Fatalf("unexpected filename: %#v", cfg)
				}
				if len(cfg.globalArgs) != 2 || cfg.globalArgs[0] != "--context" || cfg.globalArgs[1] != "demo" {
					t.Fatalf("unexpected globals: %#v", cfg.globalArgs)
				}
			},
		},
		{
			name:    "missing args",
			args:    nil,
			wantErr: errUsage,
		},
		{
			name:    "create missing file",
			args:    []string{"create"},
			wantErr: errUsage,
		},
		{
			name:    "unsupported flag",
			args:    []string{"get", "widgets", "--foo"},
			wantErr: errUsage,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := parseArgs(tt.args)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			tt.check(t, cfg)
		})
	}
}

func TestResourceMatches(t *testing.T) {
	t.Parallel()

	resource := apiResource{
		Name:         "widgets",
		SingularName: "widget",
		Kind:         "Widget",
		ShortNames:   []string{"wdg"},
	}

	for _, token := range []string{
		"widgets",
		"widget",
		"wdg",
		"widgets.example.com",
		"widget.example.com",
		"wdg.example.com",
		"widgets.v1.example.com",
		"widget.v1.example.com",
		"wdg.v1.example.com",
	} {
		if !resourceMatches(token, "example.com", "v1", resource) {
			t.Fatalf("expected %q to match", token)
		}
	}

	if resourceMatches("gadgets", "example.com", "v1", resource) {
		t.Fatal("did not expect unrelated token to match")
	}
}

func TestBuildResourcePath(t *testing.T) {
	t.Parallel()

	path, err := buildResourcePath(resourceRef{
		Group:      "example.com",
		Version:    "v1",
		Resource:   "widgets",
		Namespaced: true,
	}, "demo", false, "sample", "app=test", "metadata.name=sample", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "/apis/example.com/v1/namespaces/demo/widgets/sample?fieldSelector=metadata.name%3Dsample&labelSelector=app%3Dtest&watch=true"
	if path != want {
		t.Fatalf("got %q, want %q", path, want)
	}
}

func TestBuildResourcePathAllNamespacesError(t *testing.T) {
	t.Parallel()

	_, err := buildResourcePath(resourceRef{
		Group:      "example.com",
		Version:    "v1",
		Resource:   "widgets",
		Namespaced: true,
	}, "", true, "", "", "", false)
	if !errors.Is(err, errUsage) {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestParseManifestMeta(t *testing.T) {
	t.Parallel()

	manifest, err := parseManifestMeta([]byte(`
apiVersion: example.com/v1
kind: Widget
metadata:
  name: sample
  namespace: demo
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manifest.APIVersion != "example.com/v1" || manifest.Kind != "Widget" || manifest.Metadata.Name != "sample" || manifest.Metadata.Namespace != "demo" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}

func TestUsageMentionsAggregationEndpoints(t *testing.T) {
	t.Parallel()

	text := usage()
	for _, needle := range []string{"GET aggregation endpoint", "POST aggregation endpoint", "watch=true"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("usage missing %q", needle)
		}
	}
}
