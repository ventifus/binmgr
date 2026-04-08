package manager

import "strings"

// ExpandVars replaces ${TAG} and ${VERSION} in pattern.
// tag is the raw release tag (e.g. "v1.40.0", "knative-v1.19.5").
// ${VERSION} is the tag with a leading "v" stripped (strings.TrimPrefix(tag, "v")).
// If tag does not start with "v", ${TAG} and ${VERSION} are identical.
func ExpandVars(pattern, tag string) string {
	version := strings.TrimPrefix(tag, "v")
	return strings.NewReplacer(
		"${TAG}", tag,
		"${VERSION}", version,
	).Replace(pattern)
}
