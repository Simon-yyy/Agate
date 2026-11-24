//go:build !windows

package agent

func defaultReadExecutableProductVersion(string) string {
	return ""
}
