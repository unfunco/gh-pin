package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFormatUserAgent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		extensionVersion string
		want             string
	}{
		{
			name:             "extension only",
			extensionVersion: "v1.2.3",
			want:             "gh-pin/v1.2.3",
		},
		{
			name: "defaults to dev",
			want: "gh-pin/dev",
		},
		{
			name:             "trims whitespace",
			extensionVersion: "  v1.2.3  ",
			want:             "gh-pin/v1.2.3",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := formatUserAgent(tt.extensionVersion); got != tt.want {
				t.Fatalf("formatUserAgent(%q) = %q, want %q", tt.extensionVersion, got, tt.want)
			}
		})
	}
}

func TestFetchPinsSetsUserAgentHeader(t *testing.T) {
	t.Parallel()

	wantUserAgent := "gh-pin/v1.2.3"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.UserAgent(); got != wantUserAgent {
			t.Fatalf("User-Agent = %q, want %q", got, wantUserAgent)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"actions":[]}`))
	}))
	defer server.Close()

	a := app{
		httpClient: server.Client(),
		pinsURL:    server.URL,
		userAgent:  wantUserAgent,
	}

	pins, err := a.fetchPins(context.Background())
	if err != nil {
		t.Fatalf("fetchPins() error = %v", err)
	}
	if len(pins.Actions) != 0 {
		t.Fatalf("fetchPins() actions = %#v, want empty list", pins.Actions)
	}
}
