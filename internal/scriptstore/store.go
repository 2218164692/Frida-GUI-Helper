package scriptstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxScriptSize = 2 * 1024 * 1024

type Script struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Favorite    bool     `json:"favorite"`
	Source      string   `json:"source"`
	Origin      string   `json:"origin"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
	LastUsedAt  string   `json:"lastUsedAt"`
}

type SaveRequest struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Favorite    bool     `json:"favorite"`
	Source      string   `json:"source"`
	Origin      string   `json:"origin"`
}

type Store struct {
	path string
	now  func() time.Time
	mu   sync.Mutex
}

func New() *Store {
	configRoot, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(configRoot) == "" {
		configRoot = os.TempDir()
	}
	return &Store{
		path: filepath.Join(configRoot, "Frida-GUI-Helper", "scripts.json"),
		now:  time.Now,
	}
}

func NewAt(path string) *Store {
	return &Store{path: path, now: time.Now}
}

func (s *Store) List() ([]Script, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	scripts, err := s.read()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(scripts, func(i, j int) bool {
		if scripts[i].Favorite != scripts[j].Favorite {
			return scripts[i].Favorite
		}
		if scripts[i].LastUsedAt != scripts[j].LastUsedAt {
			return scripts[i].LastUsedAt > scripts[j].LastUsedAt
		}
		return scripts[i].UpdatedAt > scripts[j].UpdatedAt
	})
	return scripts, nil
}

func (s *Store) Save(req SaveRequest) (Script, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.Source = strings.TrimSpace(req.Source)
	req.Origin = strings.TrimSpace(req.Origin)
	if req.Name == "" {
		return Script{}, errors.New("脚本名称不能为空")
	}
	if req.Source == "" {
		return Script{}, errors.New("脚本源码不能为空")
	}
	if len([]byte(req.Source)) > maxScriptSize {
		return Script{}, fmt.Errorf("脚本源码过大: %d bytes，当前限制为 2MB", len([]byte(req.Source)))
	}
	if req.Origin == "" {
		req.Origin = "local"
	}

	scripts, err := s.read()
	if err != nil {
		return Script{}, err
	}

	now := s.now().UTC().Format(time.RFC3339)
	saved := Script{
		ID:          strings.TrimSpace(req.ID),
		Name:        req.Name,
		Description: req.Description,
		Tags:        normalizeTags(req.Tags),
		Favorite:    req.Favorite,
		Source:      req.Source,
		Origin:      req.Origin,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if saved.ID == "" {
		saved.ID = newID()
	}

	replaced := false
	for i := range scripts {
		if scripts[i].ID == saved.ID {
			saved.CreatedAt = scripts[i].CreatedAt
			saved.LastUsedAt = scripts[i].LastUsedAt
			scripts[i] = saved
			replaced = true
			break
		}
	}
	if !replaced {
		scripts = append(scripts, saved)
	}
	if err := s.write(scripts); err != nil {
		return Script{}, err
	}
	return saved, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("脚本 ID 不能为空")
	}
	scripts, err := s.read()
	if err != nil {
		return err
	}
	next := scripts[:0]
	deleted := false
	for _, script := range scripts {
		if script.ID == id {
			deleted = true
			continue
		}
		next = append(next, script)
	}
	if !deleted {
		return fmt.Errorf("脚本不存在: %s", id)
	}
	return s.write(next)
}

func (s *Store) RecordRun(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	scripts, err := s.read()
	if err != nil {
		return err
	}
	for i := range scripts {
		if scripts[i].ID == id {
			scripts[i].LastUsedAt = s.now().UTC().Format(time.RFC3339)
			return s.write(scripts)
		}
	}
	return nil
}

func (s *Store) read() ([]Script, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Script{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取本地脚本库失败: %w", err)
	}
	if len(data) == 0 {
		return []Script{}, nil
	}
	var scripts []Script
	if err := json.Unmarshal(data, &scripts); err != nil {
		return nil, fmt.Errorf("解析本地脚本库失败: %w", err)
	}
	for i := range scripts {
		scripts[i].Tags = normalizeTags(scripts[i].Tags)
	}
	return scripts, nil
}

func (s *Store) write(scripts []Script) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("创建本地脚本库目录失败: %w", err)
	}
	data, err := json.MarshalIndent(scripts, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化本地脚本库失败: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("写入本地脚本库失败: %w", err)
	}
	return nil
}

func normalizeTags(tags []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.TrimPrefix(tag, "#"))
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		normalized = append(normalized, tag)
		if len(normalized) >= 12 {
			break
		}
	}
	return normalized
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
