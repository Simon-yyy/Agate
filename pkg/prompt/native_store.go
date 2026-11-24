package prompt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"agate/pkg/git"
)

// NativePlanCandidate 记录由模型回复生成的候选方案。
type NativePlanCandidate struct {
	TurnID      string          `json:"turn_id"`
	PlanSummary string          `json:"plan_summary"`
	Diagnosis   DiagnosisStatus `json:"diagnosis"`
	Files       []string        `json:"files"`
	CreatedAt   time.Time       `json:"created_at"`
}

// NativeSnapshot 记录统一宿主会话的完整门禁快照。
type NativeSnapshot struct {
	Host             string               `json:"host"`
	SessionID        string               `json:"session_id"`
	WorkspaceID      string               `json:"workspace_id"`
	WorkspaceRoot    string               `json:"workspace_root,omitempty"`
	ActiveTurnID     string               `json:"active_turn_id"`
	ContextPath      string               `json:"context_path,omitempty"`
	DeliveryMode     string               `json:"delivery_mode,omitempty"`
	ObservedRead     bool                 `json:"observed_read"`
	ObservedReadPath string               `json:"observed_read_path,omitempty"`
	Diagnosis        DiagnosisStatus      `json:"diagnosis"`
	DiagnosisSummary string               `json:"diagnosis_summary,omitempty"`
	PlanCandidate    *NativePlanCandidate `json:"plan_candidate,omitempty"`
	PlanConfirmed    bool                 `json:"plan_confirmed"`
	ConfirmedFiles   []string             `json:"confirmed_files,omitempty"`
	ConfirmedTurnID  string               `json:"confirmed_turn_id,omitempty"`
	ConfirmEventID   string               `json:"confirm_event_id,omitempty"`
	ApprovedAt       *time.Time           `json:"approved_at,omitempty"`
	Consumed         bool                 `json:"consumed"`
	UpdatedAt        time.Time            `json:"updated_at"`
}

// NativeStore 提供基于磁盘和跨进程文件锁的会话快照持久化管理。
type NativeStore struct {
	Dir string
}

// ResolveNativeStoreDir 解析并返回多宿主统一会话存储目录。
// 规则：<gitCommonDir>/agate/requests/v2/<host>/<workspace-hash>/
func ResolveNativeStoreDir(root, host, workspace, override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		wsHash := hashText(filepath.Clean(firstText(workspace, root, ".")))
		return filepath.Abs(filepath.Join(override, "v2", host, wsHash))
	}
	common, err := git.GetGitCommonDir(firstText(root, workspace, "."))
	if err != nil {
		return "", fmt.Errorf("无法确定 Git 公共目录: %w", err)
	}
	wsHash := hashText(filepath.Clean(firstText(workspace, root, ".")))
	return filepath.Abs(filepath.Join(common, "agate", "requests", "v2", host, wsHash))
}

// NewNativeStore 创建指定目录的 NativeStore 实例。
func NewNativeStore(dir string) *NativeStore {
	return &NativeStore{Dir: dir}
}

func (s *NativeStore) snapshotFilePath(sessionID string) (string, error) {
	cleanID := strings.TrimSpace(sessionID)
	if cleanID == "" {
		return "", fmt.Errorf("会话标识不能为空")
	}
	fileKey := hashText(cleanID)
	return filepath.Join(s.Dir, fileKey+".json"), nil
}

func (s *NativeStore) lockFilePath(sessionID string) (string, error) {
	cleanID := strings.TrimSpace(sessionID)
	if cleanID == "" {
		return "", fmt.Errorf("会话标识不能为空")
	}
	return filepath.Join(s.Dir, hashText(cleanID)+".lock"), nil
}

// LoadSnapshot 加载会话快照，若不存在返回空的初始化快照。
func (s *NativeStore) LoadSnapshot(sessionID, host, workspace string) (NativeSnapshot, error) {
	path, err := s.snapshotFilePath(sessionID)
	if err != nil {
		return NativeSnapshot{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			absWS, absErr := filepath.Abs(filepath.Clean(workspace))
			if absErr != nil {
				absWS = filepath.Clean(workspace)
			}
			return NativeSnapshot{
				Host:          host,
				SessionID:     sessionID,
				WorkspaceID:   hashText(filepath.Clean(workspace)),
				WorkspaceRoot: absWS,
				Diagnosis:     DiagnosisUndiagnosed,
			}, nil
		}
		return NativeSnapshot{}, fmt.Errorf("读取快照失败: %w", err)
	}
	var snap NativeSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return NativeSnapshot{}, fmt.Errorf("快照损坏，拒绝受控写入: %w", err)
	}
	return snap, nil
}

// SaveSnapshot 原子保存会话快照；临时文件名唯一，失败时不覆盖原快照。
func (s *NativeStore) SaveSnapshot(snap NativeSnapshot) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("创建存储目录失败: %w", err)
	}
	path, err := s.snapshotFilePath(snap.SessionID)
	if err != nil {
		return err
	}
	snap.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化快照失败: %w", err)
	}
	tmp, err := os.CreateTemp(s.Dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时快照失败: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("写入临时快照失败: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil && !os.IsPermission(err) {
		return fmt.Errorf("设置临时快照权限失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时快照失败: %w", err)
	}
	if err := replaceSnapshotFile(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// replaceSnapshotFile 用临时文件替换快照。Windows 上 Rename 不能覆盖已存在文件，先把旧文件移开。
func replaceSnapshotFile(tmpName, path string) error {
	if err := os.Rename(tmpName, path); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return fmt.Errorf("原子替换快照失败: %w", err)
	}
	backup := path + ".replacing"
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("移开旧快照失败: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Rename(backup, path)
		return fmt.Errorf("原子替换快照失败: %w", err)
	}
	_ = os.Remove(backup)
	return nil
}

// WithLock 持有跨进程会话锁执行安全读写操作。
func (s *NativeStore) WithLock(sessionID string, fn func() error) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("创建存储目录失败: %w", err)
	}
	lockPath, err := s.lockFilePath(sessionID)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("打开会话锁失败: %w", err)
	}
	defer f.Close()
	if err := lockSessionFile(f); err != nil {
		return err
	}
	defer unlockSessionFile(f)
	return fn()
}
