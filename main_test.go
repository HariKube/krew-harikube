package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestBuildKubectlArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr error
	}{
		{
			name: "get",
			args: []string{"get", "pods", "-n", "default"},
			want: []string{"get", "pods", "-n", "default"},
		},
		{
			name: "list maps to get",
			args: []string{"list", "deployments"},
			want: []string{"get", "deployments"},
		},
		{
			name: "watch maps to get watch",
			args: []string{"watch", "pods"},
			want: []string{"get", "pods", "--watch=true"},
		},
		{
			name: "watch preserves extra args",
			args: []string{"watch", "pods", "-n", "default"},
			want: []string{"get", "pods", "-n", "default", "--watch=true"},
		},
		{
			name: "watch preserves resource name order",
			args: []string{"watch", "pod", "my-pod"},
			want: []string{"get", "pod", "my-pod", "--watch=true"},
		},
		{
			name: "create",
			args: []string{"create", "-f", "manifest.yaml"},
			want: []string{"create", "-f", "manifest.yaml"},
		},
		{
			name: "update maps to apply",
			args: []string{"update", "-f", "manifest.yaml"},
			want: []string{"apply", "-f", "manifest.yaml"},
		},
		{
			name: "delete",
			args: []string{"delete", "pod", "example"},
			want: []string{"delete", "pod", "example"},
		},
		{
			name:    "missing verb",
			args:    nil,
			wantErr: errUsage,
		},
		{
			name:    "unsupported verb",
			args:    []string{"patch", "pod", "example"},
			wantErr: errUsage,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := buildKubectlArgs(tt.args)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
