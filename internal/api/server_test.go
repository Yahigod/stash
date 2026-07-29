package api

import (
	"slices"
	"testing"
)

func TestDefaultPageConnectSourcesAllowConfiguredHTTPOrigins(t *testing.T) {
	sources := defaultPageConnectSources()

	for _, source := range []string{"'self'", "http:", "https:", "ws:", "wss:"} {
		if !slices.Contains(sources, source) {
			t.Errorf("default connect sources do not contain %q", source)
		}
	}
	if slices.Contains(sources, "*") {
		t.Error("default connect sources must not use an unrestricted wildcard")
	}
}
