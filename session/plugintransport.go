//go:build !windows
// +build !windows

package session

func getDefaultPluginTransport() string {
	return pluginTransportUnix
}
