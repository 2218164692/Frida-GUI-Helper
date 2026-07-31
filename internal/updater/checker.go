package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
)

const (
	ApplicationRepository = "2218164692/Frida-GUI-Helper"
	FridaRepository       = "frida/frida"

	defaultAPIBaseURL = "https://api.github.com"
	defaultUserAgent  = "Frida-GUI-Helper-update-checker"
	maxResponseBytes  = 1 << 20
	requestTimeout    = 12 * time.Second
)

// Component identifies an independently versioned part of the application.
type Component string

const (
	ComponentApplication Component = "application"
	ComponentFrida       Component = "frida"
)

// ComponentStatus is the result of checking one component against its fixed
// upstream GitHub repository.
type ComponentStatus struct {
	Component       Component `json:"component"`
	Repository      string    `json:"repository"`
	CurrentVersion  string    `json:"currentVersion"`
	LatestVersion   string    `json:"latestVersion"`
	UpdateAvailable bool      `json:"updateAvailable"`
	ReleaseURL      string    `json:"releaseUrl"`
	PublishedAt     string    `json:"publishedAt"`
	CheckedAt       string    `json:"checkedAt"`
	Error           string    `json:"error,omitempty"`
}

// Checker performs bounded, read-only checks against GitHub Releases.
type Checker struct {
	client    *http.Client
	apiBase   string
	userAgent string
	now       func() time.Time
}

// NewChecker constructs a checker with a finite request timeout.
func NewChecker() *Checker {
	return NewCheckerWithClient(nil)
}

// NewCheckerWithClient constructs a checker using client. A client without a
// timeout is copied and assigned the package's default timeout.
func NewCheckerWithClient(client *http.Client) *Checker {
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	} else {
		copy := *client
		if copy.Timeout <= 0 {
			copy.Timeout = requestTimeout
		}
		client = &copy
	}
	return &Checker{
		client:    client,
		apiBase:   defaultAPIBaseURL,
		userAgent: defaultUserAgent,
		now:       time.Now,
	}
}

// CheckApplication checks the latest Frida GUI Helper release.
func (c *Checker) CheckApplication(ctx context.Context, currentVersion string) (ComponentStatus, error) {
	return c.Check(ctx, ComponentApplication, currentVersion)
}

// CheckFrida checks the latest official Frida release.
func (c *Checker) CheckFrida(ctx context.Context, currentVersion string) (ComponentStatus, error) {
	return c.Check(ctx, ComponentFrida, currentVersion)
}

// Check resolves the fixed repository for component and reads its latest
// release. It never downloads or installs release assets.
func (c *Checker) Check(ctx context.Context, component Component, currentVersion string) (ComponentStatus, error) {
	if c == nil || c.client == nil {
		return ComponentStatus{}, errors.New("update checker is not initialized")
	}
	repository, err := repositoryFor(component)
	status := ComponentStatus{
		Component:      component,
		Repository:     repository,
		CurrentVersion: canonicalVersion(currentVersion),
		CheckedAt:      c.now().UTC().Format(time.RFC3339),
	}
	if err != nil {
		status.Error = err.Error()
		return status, err
	}

	release, err := c.latestRelease(ctx, repository)
	if err != nil {
		status.Error = err.Error()
		return status, err
	}
	status.LatestVersion = canonicalVersion(release.TagName)
	status.ReleaseURL = release.HTMLURL
	status.PublishedAt = release.PublishedAt.UTC().Format(time.RFC3339)

	if strings.TrimSpace(currentVersion) == "" {
		status.UpdateAvailable = true
		return status, nil
	}
	comparison, err := CompareVersions(release.TagName, currentVersion)
	if err != nil {
		status.Error = err.Error()
		return status, err
	}
	status.UpdateAvailable = comparison > 0
	return status, nil
}

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
}

func (c *Checker) latestRelease(ctx context.Context, repository string) (githubRelease, error) {
	endpoint := strings.TrimRight(c.apiBase, "/") + "/repos/" + repository + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return githubRelease{}, fmt.Errorf("create GitHub release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return githubRelease{}, fmt.Errorf("check GitHub release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return githubRelease{}, fmt.Errorf("GitHub release check returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxResponseBytes {
		return githubRelease{}, fmt.Errorf("GitHub release response exceeds %d bytes", maxResponseBytes)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return githubRelease{}, fmt.Errorf("read GitHub release response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return githubRelease{}, fmt.Errorf("GitHub release response exceeds %d bytes", maxResponseBytes)
	}
	var release githubRelease
	if err := json.Unmarshal(data, &release); err != nil {
		return githubRelease{}, fmt.Errorf("decode GitHub release response: %w", err)
	}
	if _, err := parseVersion(release.TagName); err != nil {
		return githubRelease{}, fmt.Errorf("invalid GitHub release tag %q: %w", release.TagName, err)
	}
	if strings.TrimSpace(release.HTMLURL) == "" {
		return githubRelease{}, errors.New("GitHub release response has no release URL")
	}
	return release, nil
}

func repositoryFor(component Component) (string, error) {
	switch component {
	case ComponentApplication:
		return ApplicationRepository, nil
	case ComponentFrida:
		return FridaRepository, nil
	default:
		return "", fmt.Errorf("unknown update component %q", component)
	}
}

// CompareVersions compares two semantic versions. It returns -1 when left is
// older, 0 when they are equivalent, and 1 when left is newer. Leading v/V is
// accepted and build metadata is ignored, as required by semantic versioning.
func CompareVersions(left, right string) (int, error) {
	a, err := parseVersion(left)
	if err != nil {
		return 0, fmt.Errorf("invalid version %q: %w", left, err)
	}
	b, err := parseVersion(right)
	if err != nil {
		return 0, fmt.Errorf("invalid version %q: %w", right, err)
	}
	for i := range a.core {
		if cmp := compareNumeric(a.core[i], b.core[i]); cmp != 0 {
			return cmp, nil
		}
	}
	if len(a.pre) == 0 && len(b.pre) == 0 {
		return 0, nil
	}
	if len(a.pre) == 0 {
		return 1, nil
	}
	if len(b.pre) == 0 {
		return -1, nil
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		av, bv := a.pre[i], b.pre[i]
		an, bn := isNumeric(av), isNumeric(bv)
		switch {
		case an && bn:
			if cmp := compareNumeric(av, bv); cmp != 0 {
				return cmp, nil
			}
		case an:
			return -1, nil
		case bn:
			return 1, nil
		default:
			if av < bv {
				return -1, nil
			}
			if av > bv {
				return 1, nil
			}
		}
	}
	if len(a.pre) < len(b.pre) {
		return -1, nil
	}
	if len(a.pre) > len(b.pre) {
		return 1, nil
	}
	return 0, nil
}

type semanticVersion struct {
	core [3]string
	pre  []string
}

func parseVersion(raw string) (semanticVersion, error) {
	value := strings.TrimSpace(raw)
	if len(value) > 1 && (value[0] == 'v' || value[0] == 'V') && value[1] >= '0' && value[1] <= '9' {
		value = value[1:]
	}
	if plus := strings.IndexByte(value, '+'); plus >= 0 {
		build := value[plus+1:]
		if !validMetadata(build) {
			return semanticVersion{}, fmt.Errorf("invalid build metadata %q", build)
		}
		value = value[:plus]
	}
	var prerelease string
	if dash := strings.IndexByte(value, '-'); dash >= 0 {
		prerelease = value[dash+1:]
		value = value[:dash]
		if prerelease == "" {
			return semanticVersion{}, errors.New("empty prerelease")
		}
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return semanticVersion{}, errors.New("expected major.minor.patch")
	}
	var parsed semanticVersion
	for i, part := range parts {
		if !validNumericIdentifier(part) {
			return semanticVersion{}, fmt.Errorf("invalid numeric identifier %q", part)
		}
		parsed.core[i] = part
	}
	if prerelease == "" {
		return parsed, nil
	}
	for _, identifier := range strings.Split(prerelease, ".") {
		if !validPrereleaseIdentifier(identifier) {
			return semanticVersion{}, fmt.Errorf("invalid prerelease identifier %q", identifier)
		}
		if isNumeric(identifier) && len(identifier) > 1 && identifier[0] == '0' {
			return semanticVersion{}, fmt.Errorf("numeric prerelease identifier %q has a leading zero", identifier)
		}
		parsed.pre = append(parsed.pre, identifier)
	}
	return parsed, nil
}

func canonicalVersion(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1 && (value[0] == 'v' || value[0] == 'V') && value[1] >= '0' && value[1] <= '9' {
		return value[1:]
	}
	return value
}

func validNumericIdentifier(value string) bool {
	if value == "" || !isNumeric(value) {
		return false
	}
	return len(value) == 1 || value[0] != '0'
}

func validPrereleaseIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(unicode.IsDigit(r) || unicode.IsLetter(r) || r == '-') || r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func validMetadata(value string) bool {
	if value == "" {
		return false
	}
	for _, identifier := range strings.Split(value, ".") {
		if !validPrereleaseIdentifier(identifier) {
			return false
		}
	}
	return true
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func compareNumeric(left, right string) int {
	left = strings.TrimLeft(left, "0")
	right = strings.TrimLeft(right, "0")
	if left == "" {
		left = "0"
	}
	if right == "" {
		right = "0"
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
