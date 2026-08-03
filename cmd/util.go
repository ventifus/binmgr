package cmd

import "strings"

// parsePackageAtVersion splits a "PACKAGE[@VERSION]" argument into its ID and
// optional version. The @VERSION suffix is recognised only when the part after
// the last '@' is non-empty and contains no '/'. If no version suffix is
// present, version is returned as an empty string.
func parsePackageAtVersion(arg string) (id, version string) {
	idx := strings.LastIndex(arg, "@")
	if idx >= 0 {
		after := arg[idx+1:]
		if after != "" && !strings.Contains(after, "/") {
			return arg[:idx], after
		}
	}
	return arg, ""
}
