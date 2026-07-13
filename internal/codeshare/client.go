package codeshare

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	defaultBaseURL   = "https://codeshare.frida.re"
	maxListResponse  = 2 * 1024 * 1024
	maxProjectSource = 2 * 1024 * 1024
	maxProjectJSON   = maxProjectSource + 512*1024
)

var projectRefPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type Client struct {
	baseURL   string
	http      *http.Client
	cacheDir  string
	trustFile string
	now       func() time.Time
}

type ProjectSummary struct {
	Ref         string `json:"ref"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Owner       string `json:"owner"`
	Slug        string `json:"slug"`
	Likes       int    `json:"likes"`
	Views       string `json:"views"`
	URL         string `json:"url"`
}

type SearchResult struct {
	Items      []ProjectSummary `json:"items"`
	Query      string           `json:"query"`
	Page       int              `json:"page"`
	TotalPages int              `json:"totalPages"`
	Source     string           `json:"source"`
	CachedAt   string           `json:"cachedAt"`
	Warning    string           `json:"warning"`
}

type Project struct {
	Ref          string `json:"ref"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Owner        string `json:"owner"`
	Slug         string `json:"slug"`
	FridaVersion string `json:"fridaVersion"`
	Likes        int    `json:"likes"`
	Source       string `json:"source"`
	Fingerprint  string `json:"fingerprint"`
	TrustState   string `json:"trustState"`
	URL          string `json:"url"`
	Origin       string `json:"origin"`
	CachedAt     string `json:"cachedAt"`
	Warning      string `json:"warning"`
}

type projectAPIResponse struct {
	ID           string `json:"id"`
	ProjectName  string `json:"project_name"`
	Description  string `json:"description"`
	Owner        string `json:"owner"`
	Slug         string `json:"slug"`
	FridaVersion string `json:"frida_version"`
	Likes        int    `json:"likes"`
	Source       string `json:"source"`
}

type cachedSearch struct {
	SavedAt    time.Time        `json:"savedAt"`
	Query      string           `json:"query"`
	Page       int              `json:"page"`
	TotalPages int              `json:"totalPages"`
	Items      []ProjectSummary `json:"items"`
}

type cachedProject struct {
	SavedAt time.Time          `json:"savedAt"`
	Data    projectAPIResponse `json:"data"`
}

type requestError struct {
	Kind       string
	StatusCode int
	Err        error
}

func (e *requestError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Kind
}

func (e *requestError) Unwrap() error { return e.Err }

func NewClient() *Client {
	cacheRoot, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(cacheRoot) == "" {
		cacheRoot = os.TempDir()
	}
	configRoot, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(configRoot) == "" {
		configRoot = cacheRoot
	}
	return newClient(
		defaultBaseURL,
		filepath.Join(cacheRoot, "Frida-GUI-Helper", "codeshare"),
		filepath.Join(configRoot, "Frida-GUI-Helper", "codeshare-trust.json"),
	)
}

func newClient(baseURL, cacheDir, trustFile string) *Client {
	parsed, _ := url.Parse(baseURL)
	allowedHost := parsed.Hostname()
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		cacheDir:  cacheDir,
		trustFile: trustFile,
		now:       time.Now,
		http: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("redirect limit exceeded")
				}
				if !strings.EqualFold(req.URL.Hostname(), allowedHost) {
					return fmt.Errorf("redirected to untrusted host %s", req.URL.Hostname())
				}
				return nil
			},
		},
	}
}

func (c *Client) Search(ctx context.Context, query string, page int) (SearchResult, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) > 120 {
		return SearchResult{}, errors.New("CodeShare 搜索词不能超过 120 个字符")
	}
	if page < 1 {
		page = 1
	}
	if page > 500 {
		return SearchResult{}, errors.New("CodeShare 页码超出允许范围")
	}

	endpoint := c.baseURL + "/browse"
	values := url.Values{}
	if query != "" {
		endpoint = c.baseURL + "/search/"
		values.Set("query", query)
	}
	if page > 1 {
		values.Set("page", strconv.Itoa(page))
	}
	if encoded := values.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}

	body, onlineErr := c.fetch(ctx, endpoint, maxListResponse, "text/html")
	if onlineErr == nil {
		items, totalPages, parseErr := parseBrowseHTML(body, c.baseURL)
		if parseErr == nil {
			cached := cachedSearch{
				SavedAt: c.now().UTC(), Query: query, Page: page,
				TotalPages: totalPages, Items: items,
			}
			result := SearchResult{
				Items: items, Query: query, Page: page, TotalPages: totalPages,
				Source: "online",
			}
			if err := c.writeJSON(c.searchCachePath(query, page), cached); err != nil {
				result.Warning = "在线数据已加载，但无法写入本地缓存: " + err.Error()
			}
			return result, nil
		}
		onlineErr = &requestError{Kind: "parse", Err: parseErr}
	}

	cached, cacheErr := c.readSearchCache(query, page)
	if cacheErr == nil {
		return SearchResult{
			Items: cached.Items, Query: query, Page: page, TotalPages: cached.TotalPages,
			Source: "cache", CachedAt: cached.SavedAt.Format(time.RFC3339),
			Warning: fmt.Sprintf("CodeShare 在线访问失败（%s），已回退到 %s 的本地缓存。", explainRequestError(onlineErr), formatLocalTime(cached.SavedAt)),
		}, nil
	}

	return SearchResult{}, fmt.Errorf(
		"CodeShare 列表加载失败：%s；本地没有可用缓存（%s）。请检查网络、系统代理或稍后重试",
		explainRequestError(onlineErr), explainCacheError(cacheErr),
	)
}

func (c *Client) GetProject(ctx context.Context, projectRef string) (Project, error) {
	projectRef, err := normalizeProjectRef(projectRef)
	if err != nil {
		return Project{}, err
	}

	endpoint := c.baseURL + "/api/project/" + projectRef + "/"
	body, onlineErr := c.fetch(ctx, endpoint, maxProjectJSON, "application/json")
	var data projectAPIResponse
	var savedAt time.Time
	origin := "online"
	warning := ""

	if onlineErr == nil {
		if err := json.Unmarshal(body, &data); err != nil {
			onlineErr = &requestError{Kind: "json", Err: err}
		} else if err := validateProject(data, projectRef); err != nil {
			onlineErr = &requestError{Kind: "json", Err: err}
		} else {
			savedAt = c.now().UTC()
			if err := c.writeJSON(c.projectCachePath(projectRef), cachedProject{SavedAt: savedAt, Data: data}); err != nil {
				warning = "源码已在线加载，但无法写入本地缓存: " + err.Error()
			}
		}
	}

	if onlineErr != nil {
		cached, cacheErr := c.readProjectCache(projectRef)
		if cacheErr != nil {
			return Project{}, fmt.Errorf(
				"CodeShare 项目 %s 加载失败：%s；本地没有可用源码缓存（%s）",
				projectRef, explainRequestError(onlineErr), explainCacheError(cacheErr),
			)
		}
		data = cached.Data
		savedAt = cached.SavedAt
		origin = "cache"
		warning = fmt.Sprintf("CodeShare 在线访问失败（%s），已加载 %s 的缓存源码。运行前仍会校验源码指纹。", explainRequestError(onlineErr), formatLocalTime(savedAt))
	}

	fingerprint := fingerprint(data.Source)
	trustState, trustWarning := c.trustState(projectRef, fingerprint)
	if trustWarning != "" {
		if warning != "" {
			warning += " "
		}
		warning += trustWarning
	}

	return Project{
		Ref: projectRef, ID: data.ID, Name: data.ProjectName, Description: data.Description,
		Owner: data.Owner, Slug: data.Slug, FridaVersion: data.FridaVersion, Likes: data.Likes,
		Source: data.Source, Fingerprint: fingerprint, TrustState: trustState,
		URL: c.baseURL + "/@" + projectRef + "/", Origin: origin,
		CachedAt: formatRFC3339(savedAt), Warning: warning,
	}, nil
}

func (c *Client) Trust(projectRef, expectedFingerprint string) error {
	projectRef, err := normalizeProjectRef(projectRef)
	if err != nil {
		return err
	}
	expectedFingerprint = strings.ToLower(strings.TrimSpace(expectedFingerprint))
	if len(expectedFingerprint) != 64 {
		return errors.New("CodeShare SHA-256 指纹格式无效")
	}
	if _, err := hex.DecodeString(expectedFingerprint); err != nil {
		return errors.New("CodeShare SHA-256 指纹格式无效")
	}

	store, err := c.readTrustStore()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("读取 CodeShare 信任记录失败: %w", err)
	}
	if store == nil {
		store = make(map[string]string)
	}
	store[projectRef] = expectedFingerprint
	if err := c.writeJSON(c.trustFile, store); err != nil {
		return fmt.Errorf("保存 CodeShare 信任记录失败: %w", err)
	}
	return nil
}

func (c *Client) fetch(ctx context.Context, endpoint string, limit int64, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, &requestError{Kind: "request", Err: err}
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "Frida-GUI-Helper/1.0 (CodeShare client)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, classifyNetworkError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32*1024))
		return nil, &requestError{Kind: "http", StatusCode: resp.StatusCode, Err: fmt.Errorf("HTTP %d", resp.StatusCode)}
	}

	reader := io.LimitReader(resp.Body, limit+1)
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, &requestError{Kind: "read", Err: err}
	}
	if int64(len(body)) > limit {
		return nil, &requestError{Kind: "size", Err: fmt.Errorf("response exceeds %d bytes", limit)}
	}
	return body, nil
}

func parseBrowseHTML(body []byte, baseURL string) ([]ProjectSummary, int, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, 0, fmt.Errorf("无法解析 HTML: %w", err)
	}

	articles := findElements(doc, "article")
	if len(articles) == 0 {
		return nil, 0, errors.New("页面中未找到项目列表，CodeShare 页面结构可能已经变化")
	}

	items := make([]ProjectSummary, 0, len(articles))
	for _, article := range articles {
		h2 := firstElement(article, "h2")
		link := firstElement(h2, "a")
		href := attr(link, "href")
		ref, owner, slug, ok := projectRefFromURL(href)
		if !ok {
			continue
		}

		likes, views := parseStats(textContent(firstElement(article, "h3")))
		projectURL := strings.TrimRight(baseURL, "/") + "/@" + ref + "/"
		items = append(items, ProjectSummary{
			Ref: ref, Name: cleanText(textContent(h2)), Description: cleanText(textContent(firstElement(article, "p"))),
			Owner: owner, Slug: slug, Likes: likes, Views: views, URL: projectURL,
		})
	}
	if len(items) == 0 {
		if strings.Contains(strings.ToLower(cleanText(textContent(doc))), "no results found") {
			return []ProjectSummary{}, 1, nil
		}
		return nil, 0, errors.New("找到项目容器但无法识别项目链接，CodeShare 页面结构可能已经变化")
	}

	totalPages := 1
	for _, link := range findElements(doc, "a") {
		href := attr(link, "href")
		parsed, err := url.Parse(href)
		if err != nil {
			continue
		}
		candidate, err := strconv.Atoi(parsed.Query().Get("page"))
		if err == nil && candidate > totalPages {
			totalPages = candidate
		}
	}
	return items, totalPages, nil
}

func validateProject(data projectAPIResponse, expectedRef string) error {
	actualRef := data.Owner + "/" + data.Slug
	if actualRef != expectedRef {
		return fmt.Errorf("项目标识不匹配: expected %s, got %s", expectedRef, actualRef)
	}
	if strings.TrimSpace(data.ProjectName) == "" {
		return errors.New("API 返回的项目名称为空")
	}
	if strings.TrimSpace(data.Source) == "" {
		return errors.New("API 返回的脚本源码为空")
	}
	if len(data.Source) > maxProjectSource {
		return fmt.Errorf("脚本源码超过 %d 字节限制", maxProjectSource)
	}
	return nil
}

func (c *Client) trustState(projectRef, currentFingerprint string) (string, string) {
	store, err := c.readTrustStore()
	if errors.Is(err, os.ErrNotExist) {
		return "new", ""
	}
	if err != nil {
		return "new", "CodeShare 信任记录无法读取，本次按未信任脚本处理: " + err.Error()
	}
	trustedFingerprint := strings.ToLower(strings.TrimSpace(store[projectRef]))
	if trustedFingerprint == "" {
		return "new", ""
	}
	if trustedFingerprint == currentFingerprint {
		return "trusted", ""
	}
	return "changed", "CodeShare 源码指纹与上次信任记录不同，请检查变更后重新确认。"
}

func (c *Client) readTrustStore() (map[string]string, error) {
	var store map[string]string
	if err := readJSON(c.trustFile, &store); err != nil {
		return nil, err
	}
	return store, nil
}

func (c *Client) readSearchCache(query string, page int) (cachedSearch, error) {
	var cached cachedSearch
	if err := readJSON(c.searchCachePath(query, page), &cached); err != nil {
		return cachedSearch{}, err
	}
	if cached.SavedAt.IsZero() || cached.Query != query || cached.Page != page || (query == "" && len(cached.Items) == 0) {
		return cachedSearch{}, errors.New("缓存内容不完整")
	}
	return cached, nil
}

func (c *Client) readProjectCache(projectRef string) (cachedProject, error) {
	var cached cachedProject
	if err := readJSON(c.projectCachePath(projectRef), &cached); err != nil {
		return cachedProject{}, err
	}
	if cached.SavedAt.IsZero() {
		return cachedProject{}, errors.New("缓存时间缺失")
	}
	if err := validateProject(cached.Data, projectRef); err != nil {
		return cachedProject{}, fmt.Errorf("缓存内容无效: %w", err)
	}
	return cached, nil
}

func (c *Client) searchCachePath(query string, page int) string {
	key := fmt.Sprintf("%s\n%d", query, page)
	return filepath.Join(c.cacheDir, "search-"+shortHash(key)+".json")
}

func (c *Client) projectCachePath(projectRef string) string {
	return filepath.Join(c.cacheDir, "project-"+shortHash(projectRef)+".json")
}

func (c *Client) writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".codeshare-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		_ = os.Remove(path)
		return os.Rename(tempName, path)
	}
	return nil
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("JSON 损坏: %w", err)
	}
	return nil
}

func classifyNetworkError(err error) error {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &requestError{Kind: "timeout", Err: err}
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return &requestError{Kind: "dns", Err: err}
	}
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return &requestError{Kind: "tls", Err: err}
	}
	var certificateInvalid x509.CertificateInvalidError
	if errors.As(err, &certificateInvalid) {
		return &requestError{Kind: "tls", Err: err}
	}
	var recordHeader tls.RecordHeaderError
	if errors.As(err, &recordHeader) {
		return &requestError{Kind: "tls", Err: err}
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "certificate") || strings.Contains(message, "tls") || strings.Contains(message, "x509") {
		return &requestError{Kind: "tls", Err: err}
	}
	if strings.Contains(message, "proxy") {
		return &requestError{Kind: "proxy", Err: err}
	}
	return &requestError{Kind: "network", Err: err}
}

func explainRequestError(err error) string {
	var reqErr *requestError
	if !errors.As(err, &reqErr) {
		if err == nil {
			return "未知错误"
		}
		return err.Error()
	}
	if reqErr.StatusCode != 0 {
		switch reqErr.StatusCode {
		case http.StatusForbidden:
			return "HTTP 403，CodeShare 拒绝访问，可能被代理、网络策略或站点防护拦截"
		case http.StatusNotFound:
			return "HTTP 404，项目不存在、已删除或链接已变化"
		case http.StatusTooManyRequests:
			return "HTTP 429，请求过于频繁，请稍后重试"
		default:
			if reqErr.StatusCode >= 500 {
				return fmt.Sprintf("HTTP %d，CodeShare 服务暂时异常", reqErr.StatusCode)
			}
			return fmt.Sprintf("HTTP %d，CodeShare 返回了非成功状态", reqErr.StatusCode)
		}
	}
	switch reqErr.Kind {
	case "timeout":
		return "连接超时，请检查网络或代理"
	case "dns":
		return "域名解析失败，请检查 DNS 和网络连接"
	case "tls":
		return "TLS 证书校验失败，请检查系统时间、代理证书或 HTTPS 检查软件"
	case "proxy":
		return "系统代理连接失败，请检查代理设置"
	case "parse":
		return "CodeShare 页面结构无法识别，站点可能已经更新"
	case "json":
		return "CodeShare API 返回了无效数据"
	case "size":
		return "CodeShare 返回内容超过安全大小限制"
	case "read":
		return "读取 CodeShare 响应时连接中断"
	default:
		return "网络连接失败: " + reqErr.Error()
	}
}

func explainCacheError(err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return "尚未生成缓存"
	}
	return err.Error()
}

func normalizeProjectRef(value string) (string, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "@")
	value = strings.Trim(value, "/")
	if !projectRefPattern.MatchString(value) {
		return "", errors.New("CodeShare 项目标识无效，应为 author/project-slug")
	}
	return value, nil
}

func projectRefFromURL(value string) (ref, owner, slug string, ok bool) {
	parsed, err := url.Parse(value)
	if err != nil {
		return "", "", "", false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "@") {
		return "", "", "", false
	}
	owner = strings.TrimPrefix(parts[0], "@")
	slug = parts[1]
	ref, err = normalizeProjectRef(owner + "/" + slug)
	return ref, owner, slug, err == nil
}

func parseStats(value string) (int, string) {
	parts := strings.Split(cleanText(value), "|")
	likes := 0
	views := ""
	if len(parts) > 0 {
		likes, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
	}
	if len(parts) > 1 {
		views = strings.TrimSpace(parts[1])
	}
	return likes, views
}

func findElements(root *html.Node, tag string) []*html.Node {
	if root == nil {
		return nil
	}
	var nodes []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && strings.EqualFold(node.Data, tag) {
			nodes = append(nodes, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return nodes
}

func firstElement(root *html.Node, tag string) *html.Node {
	nodes := findElements(root, tag)
	if len(nodes) == 0 {
		return nil
	}
	return nodes[0]
}

func textContent(root *html.Node) string {
	if root == nil {
		return ""
	}
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			builder.WriteString(node.Data)
			builder.WriteByte(' ')
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return builder.String()
}

func attr(node *html.Node, name string) string {
	if node == nil {
		return ""
	}
	for _, item := range node.Attr {
		if strings.EqualFold(item.Key, name) {
			return item.Val
		}
	}
	return ""
}

func cleanText(value string) string { return strings.Join(strings.Fields(value), " ") }

func fingerprint(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:12])
}

func formatRFC3339(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func formatLocalTime(value time.Time) string {
	if value.IsZero() {
		return "未知时间"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}
