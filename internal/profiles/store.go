package profiles

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	configDirectory   = "Frida-GUI-Helper"
	configFile        = "profiles.json"
	storeVersion      = 1
	maxProfiles       = 200
	maxScriptSize     = 2 * 1024 * 1024
	defaultServerPath = "/data/local/tmp/frida-server"
)

// ErrNotFound indicates that a requested profile does not exist.
var ErrNotFound = errors.New("运行配置不存在")

var errUnsupportedStoreVersion = errors.New("不支持的运行配置版本")

// Profile is a reusable Frida launch configuration.
type Profile struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	DeviceSerial string            `json:"deviceSerial"`
	PackageName  string            `json:"packageName"`
	Target       string            `json:"target"`
	Mode         string            `json:"mode"`
	TargetKind   string            `json:"targetKind"`
	ScriptName   string            `json:"scriptName"`
	ScriptSource string            `json:"scriptSource"`
	ScriptID     string            `json:"scriptID"`
	Variables    map[string]string `json:"variables"`
	ServerPath   string            `json:"serverPath"`
	CreatedAt    string            `json:"createdAt"`
	UpdatedAt    string            `json:"updatedAt"`
	LastRunAt    string            `json:"lastRunAt"`
}

// SaveRequest contains the editable fields of a profile. If ID is empty, Save
// creates a new profile; otherwise it replaces the matching profile.
type SaveRequest struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	DeviceSerial string            `json:"deviceSerial"`
	PackageName  string            `json:"packageName"`
	Target       string            `json:"target"`
	Mode         string            `json:"mode"`
	TargetKind   string            `json:"targetKind"`
	ScriptName   string            `json:"scriptName"`
	ScriptSource string            `json:"scriptSource"`
	ScriptID     string            `json:"scriptID"`
	Variables    map[string]string `json:"variables"`
	ServerPath   string            `json:"serverPath"`
}

type diskState struct {
	Version       int       `json:"version"`
	LastProfileID string    `json:"lastProfileID"`
	Profiles      []Profile `json:"profiles"`
}

// Store persists profiles in the current user's configuration directory. It
// is safe for concurrent use by multiple goroutines.
type Store struct {
	path string
	now  func() time.Time
	mu   sync.Mutex
}

// New creates a store at %AppData%/Frida-GUI-Helper/profiles.json on Windows.
func New() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("定位用户配置目录失败: %w", err)
	}
	return NewAt(filepath.Join(dir, configDirectory, configFile)), nil
}

// NewAt creates a store at path. It is intended for tests and portable
// environments; production callers should normally use New.
func NewAt(path string) *Store {
	if strings.TrimSpace(path) == "" {
		return &Store{now: time.Now}
	}
	return &Store{path: filepath.Clean(path), now: time.Now}
}

// Path returns the profiles file location.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// List returns profiles ordered by most recent run, then most recent update.
func (s *Store) List() ([]Profile, error) {
	if err := s.validateStore(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.readLocked()
	if err != nil {
		return nil, err
	}
	profiles := cloneProfiles(state.Profiles)
	sort.SliceStable(profiles, func(i, j int) bool {
		if profiles[i].LastRunAt != profiles[j].LastRunAt {
			return profiles[i].LastRunAt > profiles[j].LastRunAt
		}
		if profiles[i].UpdatedAt != profiles[j].UpdatedAt {
			return profiles[i].UpdatedAt > profiles[j].UpdatedAt
		}
		return strings.ToLower(profiles[i].Name) < strings.ToLower(profiles[j].Name)
	})
	return profiles, nil
}

// Get returns one profile by ID.
func (s *Store) Get(id string) (Profile, error) {
	if err := s.validateStore(); err != nil {
		return Profile{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Profile{}, errors.New("运行配置 ID 不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return Profile{}, err
	}
	for _, profile := range state.Profiles {
		if profile.ID == id {
			return cloneProfile(profile), nil
		}
	}
	return Profile{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}

// Save creates or updates a profile. Empty mode, target kind, name, variables,
// and server path receive safe defaults.
func (s *Store) Save(req SaveRequest) (Profile, error) {
	if err := s.validateStore(); err != nil {
		return Profile{}, err
	}
	normalized, err := normalizeRequest(req)
	if err != nil {
		return Profile{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return Profile{}, err
	}

	now := s.currentTime().UTC().Format(time.RFC3339)
	saved := Profile{
		ID:           normalized.ID,
		Name:         normalized.Name,
		DeviceSerial: normalized.DeviceSerial,
		PackageName:  normalized.PackageName,
		Target:       normalized.Target,
		Mode:         normalized.Mode,
		TargetKind:   normalized.TargetKind,
		ScriptName:   normalized.ScriptName,
		ScriptSource: normalized.ScriptSource,
		ScriptID:     normalized.ScriptID,
		Variables:    cloneVariables(normalized.Variables),
		ServerPath:   normalized.ServerPath,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if saved.ID == "" {
		saved.ID = newID()
	}

	replaced := false
	for i := range state.Profiles {
		if state.Profiles[i].ID != saved.ID {
			continue
		}
		saved.CreatedAt = state.Profiles[i].CreatedAt
		saved.LastRunAt = state.Profiles[i].LastRunAt
		state.Profiles[i] = saved
		replaced = true
		break
	}
	if !replaced {
		if normalized.ID != "" {
			return Profile{}, fmt.Errorf("%w: %s", ErrNotFound, normalized.ID)
		}
		if len(state.Profiles) >= maxProfiles {
			return Profile{}, fmt.Errorf("运行配置数量已达到上限: %d", maxProfiles)
		}
		state.Profiles = append(state.Profiles, saved)
	}
	if err := s.writeLocked(state); err != nil {
		return Profile{}, err
	}
	return cloneProfile(saved), nil
}

// Delete removes a profile and clears the last-run pointer when necessary.
func (s *Store) Delete(id string) error {
	if err := s.validateStore(); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("运行配置 ID 不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return err
	}
	next := make([]Profile, 0, len(state.Profiles))
	deleted := false
	for _, profile := range state.Profiles {
		if profile.ID == id {
			deleted = true
			continue
		}
		next = append(next, profile)
	}
	if !deleted {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	state.Profiles = next
	if state.LastProfileID == id {
		state.LastProfileID = ""
	}
	return s.writeLocked(state)
}

// Last returns the last successfully marked profile. It returns nil when no
// profile has been run yet.
func (s *Store) Last() (*Profile, error) {
	if err := s.validateStore(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return nil, err
	}
	if state.LastProfileID == "" {
		return nil, nil
	}
	for _, profile := range state.Profiles {
		if profile.ID == state.LastProfileID {
			copy := cloneProfile(profile)
			return &copy, nil
		}
	}

	// Repair a stale pointer left by an older or manually edited file.
	state.LastProfileID = ""
	if err := s.writeLocked(state); err != nil {
		return nil, err
	}
	return nil, nil
}

// MarkRun records that a profile was used and makes it the last-run profile.
func (s *Store) MarkRun(id string) (Profile, error) {
	if err := s.validateStore(); err != nil {
		return Profile{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Profile{}, errors.New("运行配置 ID 不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked()
	if err != nil {
		return Profile{}, err
	}
	for i := range state.Profiles {
		if state.Profiles[i].ID != id {
			continue
		}
		state.Profiles[i].LastRunAt = s.currentTime().UTC().Format(time.RFC3339)
		state.LastProfileID = id
		if err := s.writeLocked(state); err != nil {
			return Profile{}, err
		}
		return cloneProfile(state.Profiles[i]), nil
	}
	return Profile{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}

func (s *Store) validateStore() error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return errors.New("运行配置存储路径为空")
	}
	return nil
}

func (s *Store) currentTime() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

func (s *Store) readLocked() (diskState, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		backupPath := s.path + ".previous"
		backupData, backupErr := os.ReadFile(backupPath)
		if errors.Is(backupErr, os.ErrNotExist) {
			return emptyState(), nil
		}
		if backupErr != nil {
			return diskState{}, fmt.Errorf("读取运行配置恢复文件失败: %w", backupErr)
		}
		state, decodeErr := decodeState(backupData)
		if decodeErr != nil {
			return diskState{}, fmt.Errorf("运行配置主文件缺失且恢复文件无效: %w", decodeErr)
		}
		if renameErr := os.Rename(backupPath, s.path); renameErr != nil {
			return diskState{}, fmt.Errorf("恢复上次运行配置失败: %w", renameErr)
		}
		return state, nil
	}
	if err != nil {
		return diskState{}, fmt.Errorf("读取运行配置失败: %w", err)
	}

	state, err := decodeState(data)
	if err == nil {
		return state, nil
	}
	if errors.Is(err, errUnsupportedStoreVersion) {
		return diskState{}, err
	}
	if recoverErr := s.recoverCorruptLocked(); recoverErr != nil {
		return diskState{}, fmt.Errorf("恢复损坏的运行配置失败（%v）: %w", err, recoverErr)
	}
	return emptyState(), nil
}

func decodeState(data []byte) (diskState, error) {
	var state diskState
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&state); err != nil {
		return diskState{}, fmt.Errorf("解析运行配置失败: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("包含多个 JSON 值")
		}
		return diskState{}, fmt.Errorf("解析运行配置失败: %w", err)
	}
	if state.Version != storeVersion {
		return diskState{}, fmt.Errorf("%w: %d", errUnsupportedStoreVersion, state.Version)
	}
	if state.Profiles == nil {
		state.Profiles = []Profile{}
	}
	seen := make(map[string]struct{}, len(state.Profiles))
	for i := range state.Profiles {
		profile := &state.Profiles[i]
		if strings.TrimSpace(profile.ID) == "" {
			return diskState{}, errors.New("运行配置缺少 ID")
		}
		if _, exists := seen[profile.ID]; exists {
			return diskState{}, fmt.Errorf("运行配置 ID 重复: %s", profile.ID)
		}
		seen[profile.ID] = struct{}{}
		if profile.Variables == nil {
			profile.Variables = map[string]string{}
		}
	}
	return state, nil
}

func (s *Store) writeLocked(state diskState) error {
	state.Version = storeVersion
	if state.Profiles == nil {
		state.Profiles = []Profile{}
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建运行配置目录失败: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".profiles-*.tmp")
	if err != nil {
		return fmt.Errorf("创建运行配置临时文件失败: %w", err)
	}
	tempPath := temp.Name()
	keepTemp := true
	defer func() {
		if keepTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("设置运行配置文件权限失败: %w", err)
	}
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		_ = temp.Close()
		return fmt.Errorf("序列化运行配置失败: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("刷新运行配置失败: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("关闭运行配置临时文件失败: %w", err)
	}

	backupPath := s.path + ".previous"
	_ = os.Remove(backupPath)
	hadOriginal := false
	if _, err := os.Stat(s.path); err == nil {
		if err := os.Rename(s.path, backupPath); err != nil {
			return fmt.Errorf("暂存旧运行配置失败: %w", err)
		}
		hadOriginal = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("检查旧运行配置失败: %w", err)
	}
	if err := os.Rename(tempPath, s.path); err != nil {
		if hadOriginal {
			_ = os.Rename(backupPath, s.path)
		}
		return fmt.Errorf("替换运行配置失败: %w", err)
	}
	if hadOriginal {
		_ = os.Remove(backupPath)
	}
	keepTemp = false
	return nil
}

func (s *Store) recoverCorruptLocked() error {
	backup := fmt.Sprintf("%s.corrupt-%s", s.path, s.currentTime().UTC().Format("20060102T150405.000000000Z"))
	if err := os.Rename(s.path, backup); err != nil {
		return fmt.Errorf("保留损坏文件失败: %w", err)
	}
	if err := s.writeLocked(emptyState()); err != nil {
		_ = os.Rename(backup, s.path)
		return err
	}
	return nil
}

func normalizeRequest(req SaveRequest) (SaveRequest, error) {
	req.ID = strings.TrimSpace(req.ID)
	req.Name = strings.TrimSpace(req.Name)
	req.DeviceSerial = strings.TrimSpace(req.DeviceSerial)
	req.PackageName = strings.TrimSpace(req.PackageName)
	req.Target = strings.TrimSpace(req.Target)
	req.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	req.TargetKind = strings.ToLower(strings.TrimSpace(req.TargetKind))
	req.ScriptName = strings.TrimSpace(req.ScriptName)
	req.ScriptID = strings.TrimSpace(req.ScriptID)
	req.ServerPath = strings.TrimSpace(req.ServerPath)

	if req.Mode == "" {
		req.Mode = "attach"
	}
	if req.Mode != "attach" && req.Mode != "spawn" {
		return SaveRequest{}, fmt.Errorf("不支持的运行模式: %s", req.Mode)
	}
	if req.TargetKind == "" {
		req.TargetKind = "name"
	}
	if req.Mode == "spawn" {
		req.TargetKind = "package"
	}
	if req.TargetKind != "pid" && req.TargetKind != "name" && req.TargetKind != "package" {
		return SaveRequest{}, fmt.Errorf("不支持的目标类型: %s", req.TargetKind)
	}
	if req.Target == "" && req.PackageName != "" {
		req.Target = req.PackageName
	}
	if req.PackageName == "" && req.TargetKind == "package" {
		req.PackageName = req.Target
	}
	if req.ServerPath == "" {
		req.ServerPath = defaultServerPath
	}
	if req.Name == "" {
		req.Name = firstNonEmpty(req.PackageName, req.Target, req.ScriptName, "未命名配置")
	}
	if len([]rune(req.Name)) > 200 {
		return SaveRequest{}, errors.New("运行配置名称不能超过 200 个字符")
	}
	if len([]byte(req.ScriptSource)) > maxScriptSize {
		return SaveRequest{}, fmt.Errorf("脚本源码过大: %d bytes，当前限制为 2MB", len([]byte(req.ScriptSource)))
	}
	if len(req.Variables) > 100 {
		return SaveRequest{}, errors.New("脚本变量不能超过 100 个")
	}
	normalizedVariables := make(map[string]string, len(req.Variables))
	for key, value := range req.Variables {
		key = strings.TrimSpace(key)
		if key == "" {
			return SaveRequest{}, errors.New("脚本变量名称不能为空")
		}
		if len([]rune(key)) > 100 {
			return SaveRequest{}, fmt.Errorf("脚本变量名称过长: %s", key)
		}
		if len([]byte(value)) > 64*1024 {
			return SaveRequest{}, fmt.Errorf("脚本变量值过大: %s", key)
		}
		normalizedVariables[key] = value
	}
	req.Variables = normalizedVariables
	return req, nil
}

func emptyState() diskState {
	return diskState{Version: storeVersion, Profiles: []Profile{}}
}

func cloneProfiles(profiles []Profile) []Profile {
	cloned := make([]Profile, len(profiles))
	for i, profile := range profiles {
		cloned[i] = cloneProfile(profile)
	}
	return cloned
}

func cloneProfile(profile Profile) Profile {
	profile.Variables = cloneVariables(profile.Variables)
	return profile
}

func cloneVariables(variables map[string]string) map[string]string {
	cloned := make(map[string]string, len(variables))
	for key, value := range variables {
		cloned[key] = value
	}
	return cloned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
