//go:build windows

package agent

import (
	"fmt"
	"os"
	"path"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const processQueryLimitedInformation = 0x1000

var (
	modkernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procQueryFullProcessImageNameW = modkernel32.NewProc("QueryFullProcessImageNameW")
)

type processQuerier interface {
	snapshot() (map[int]int, error)
	query(pid int) (image string, created time.Time, err error)
}

type systemProcessQuerier struct{}

// GetAncestors 用一次进程快照沿父 PID 采集有限祖先链，只读取可执行文件路径和创建时间。
func (d *DefaultProcessInspector) GetAncestors(pid int, maxDepth int) ([]ProcessInfo, error) {
	return collectWindowsAncestors(pid, maxDepth, systemProcessQuerier{})
}

func collectWindowsAncestors(startPID int, maxDepth int, q processQuerier) ([]ProcessInfo, error) {
	maxDepth = normalizeAncestorDepth(maxDepth)
	pid := startPID
	if pid <= 0 {
		pid = os.Getpid()
	}

	parents, err := q.snapshot()
	if err != nil {
		return nil, err
	}

	var chain []ProcessInfo
	visited := make(map[int]struct{})
	var previousCreated time.Time
	hasPrevious := false

	for len(chain) < maxDepth {
		if pid <= 0 {
			break
		}
		if _, seen := visited[pid]; seen {
			return chain, fmt.Errorf("祖先进程链成环，已停止追踪")
		}
		visited[pid] = struct{}{}

		ppid, known := parents[pid]
		image, created, qerr := q.query(pid)
		if qerr != nil {
			return chain, fmt.Errorf("进程 %d 元数据不可读: %w", pid, qerr)
		}
		if strings.TrimSpace(image) == "" {
			return chain, fmt.Errorf("进程 %d 没有可用路径，已停止追踪", pid)
		}
		if hasPrevious && created.After(previousCreated) {
			return chain, fmt.Errorf("进程 %d 创建时间晚于子进程，疑似 PID 复用，已停止追踪", pid)
		}

		chain = append(chain, ProcessInfo{
			PID:  pid,
			PPID: ppid,
			Path: image,
			Name: path.Base(strings.ReplaceAll(image, "\\", "/")),
		})

		if !known {
			return chain, fmt.Errorf("进程 %d 的父进程关系无法核验，已停止追踪", pid)
		}
		if ppid <= 0 || ppid == pid {
			break
		}
		previousCreated = created
		hasPrevious = true
		pid = ppid
	}

	return chain, nil
}

func (systemProcessQuerier) snapshot() (map[int]int, error) {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("进程快照失败: %w", err)
	}
	defer syscall.CloseHandle(snap)

	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := syscall.Process32First(snap, &entry); err != nil {
		return nil, fmt.Errorf("进程快照失败: %w", err)
	}

	parents := make(map[int]int)
	for {
		parents[int(entry.ProcessID)] = int(entry.ParentProcessID)
		if err := syscall.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	return parents, nil
}

func (systemProcessQuerier) query(pid int) (string, time.Time, error) {
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return "", time.Time{}, err
	}
	defer syscall.CloseHandle(handle)

	image, err := queryFullProcessImageName(handle)
	if err != nil {
		return "", time.Time{}, err
	}
	var created, exited, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
		return "", time.Time{}, err
	}
	return image, time.Unix(0, created.Nanoseconds()), nil
}

func queryFullProcessImageName(handle syscall.Handle) (string, error) {
	buf := make([]uint16, syscall.MAX_PATH)
	for {
		if len(buf) == 0 || len(buf) > syscall.MAX_LONG_PATH {
			return "", fmt.Errorf("可执行文件路径超出上限")
		}
		size := uint32(len(buf))
		r, _, callErr := procQueryFullProcessImageNameW.Call(
			uintptr(handle),
			0,
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(unsafe.Pointer(&size)),
		)
		if r != 0 {
			if size > uint32(len(buf)) {
				size = uint32(len(buf))
			}
			return syscall.UTF16ToString(buf[:size]), nil
		}
		errno, _ := callErr.(syscall.Errno)
		if errno == syscall.Errno(122) && len(buf) < syscall.MAX_LONG_PATH {
			next := len(buf) * 2
			if next > syscall.MAX_LONG_PATH {
				next = syscall.MAX_LONG_PATH
			}
			if next == len(buf) {
				return "", fmt.Errorf("可执行文件路径超出上限")
			}
			buf = make([]uint16, next)
			continue
		}
		if callErr != nil {
			return "", callErr
		}
		return "", fmt.Errorf("无法读取进程可执行文件路径")
	}
}
