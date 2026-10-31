package imageref

import (
	"strings"
	"testing"
)

func TestVersionLabel(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)

	tests := []struct {
		name  string
		image string
		want  string
	}{
		{name: "tag", image: "ghcr.io/kubebrowser/browser-server:0.0.2", want: "0.0.2"},
		{name: "no registry", image: "browser-server:1.0", want: "1.0"},
		{name: "port and tag", image: "registry:5000/kubebrowser/browser-server:1.0", want: "1.0"},
		{name: "port without tag", image: "registry:5000/kubebrowser/browser-server", want: ""},
		{name: "no tag", image: "ghcr.io/kubebrowser/browser-server", want: ""},
		{name: "digest", image: "ghcr.io/kubebrowser/browser-server@" + digest, want: ""},
		{name: "tag and digest", image: "ghcr.io/kubebrowser/browser-server:0.0.2@" + digest, want: "0.0.2"},
		{name: "port, tag and digest", image: "registry:5000/kubebrowser/browser-server:v1@" + digest, want: "v1"},
		{name: "tag too long for a label", image: "repo:" + strings.Repeat("1", 64), want: ""},
		{name: "tag not a valid label", image: "repo:-dev", want: ""},
		{name: "empty", image: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VersionLabel(tt.image); got != tt.want {
				t.Errorf("VersionLabel(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}
