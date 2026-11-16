//go:build !windows && !linux

package agent

// GetAncestors 在没有原生采集实现的平台上安全降级，不请求提权，不执行外部命令。
func (d *DefaultProcessInspector) GetAncestors(pid int, maxDepth int) ([]ProcessInfo, error) {
	return nil, nil
}
