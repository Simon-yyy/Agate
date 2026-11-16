//go:build linux

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// GetAncestors 沿 /proc 采集有限祖先进程链，保持原有 Linux 行为。
func (d *DefaultProcessInspector) GetAncestors(pid int, maxDepth int) ([]ProcessInfo, error) {
	maxDepth = normalizeAncestorDepth(maxDepth)

	var chain []ProcessInfo
	currPID := pid
	if currPID <= 0 {
		currPID = os.Getpid()
	}

	for depth := 0; depth < maxDepth; depth++ {
		if currPID <= 1 {
			break
		}

		info, err := readLinuxProcInfo(currPID)
		if err != nil {
			break
		}

		chain = append(chain, info)
		if info.PPID <= 1 || info.PPID == currPID {
			break
		}
		currPID = info.PPID
	}

	return chain, nil
}

// readLinuxProcInfo 读取指定 PID 的父进程及可执行文件路径（仅通过 /proc 内部接口）。
func readLinuxProcInfo(pid int) (ProcessInfo, error) {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return ProcessInfo{}, err
	}

	statStr := string(data)
	closeParen := strings.LastIndex(statStr, ")")
	if closeParen < 0 || closeParen+2 >= len(statStr) {
		return ProcessInfo{}, fmt.Errorf("无法解析 /proc/%d/stat", pid)
	}

	comm := ""
	openParen := strings.Index(statStr, "(")
	if openParen >= 0 && openParen < closeParen {
		comm = statStr[openParen+1 : closeParen]
	}

	rest := strings.Fields(statStr[closeParen+2:])
	if len(rest) < 2 {
		return ProcessInfo{}, fmt.Errorf("/proc/%d/stat 字段不完整", pid)
	}

	ppid, err := strconv.Atoi(rest[1])
	if err != nil {
		ppid = 0
	}

	exePath := ""
	if target, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		exePath = target
	}

	name := comm
	if exePath != "" {
		name = filepath.Base(exePath)
	}

	return ProcessInfo{
		PID:  pid,
		PPID: ppid,
		Path: exePath,
		Name: name,
	}, nil
}
