// Package imageref holds helpers for working with container image references.
package imageref

import (
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// VersionLabel returns the tag of an image reference, suitable for use as the
// value of the app.kubernetes.io/version label.
//
// Registry ports ("registry:5000/repo:1.0") and digests ("repo:1.0@sha256:...")
// are handled. An empty string is returned when the reference has no tag
// (e.g. digest-only references) or when the tag is not a valid label value
// (e.g. longer than 63 characters).
func VersionLabel(image string) string {
	// Drop the digest, if any.
	if i := strings.Index(image, "@"); i >= 0 {
		image = image[:i]
	}

	// The tag can only appear after the last path segment, which keeps a
	// registry port from being mistaken for a tag.
	lastSegment := image[strings.LastIndex(image, "/")+1:]
	i := strings.LastIndex(lastSegment, ":")
	if i < 0 {
		return ""
	}

	tag := lastSegment[i+1:]
	if len(validation.IsValidLabelValue(tag)) > 0 {
		return ""
	}
	return tag
}
