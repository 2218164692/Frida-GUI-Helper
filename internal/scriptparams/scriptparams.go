package scriptparams

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const metadataMarker = "@frida-gui"

var (
	parameterNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
	placeholderPattern   = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)
)

// ParameterType describes the input control and validation applied to a
// parameter.
type ParameterType string

const (
	TypeString  ParameterType = "string"
	TypeNumber  ParameterType = "number"
	TypeBoolean ParameterType = "boolean"
	TypeSelect  ParameterType = "select"
)

// Metadata is the JSON-serializable description of a script parameter form.
type Metadata struct {
	Name        string      `json:"name,omitempty"`
	Description string      `json:"description,omitempty"`
	Parameters  []Parameter `json:"parameters"`
}

// Parameter describes one script parameter. HasDefault distinguishes an
// omitted default from a valid false, zero, or empty-string default.
type Parameter struct {
	Name        string        `json:"name"`
	Label       string        `json:"label"`
	Description string        `json:"description,omitempty"`
	Placeholder string        `json:"placeholder,omitempty"`
	Type        ParameterType `json:"type"`
	Required    bool          `json:"required"`
	Default     any           `json:"default,omitempty"`
	HasDefault  bool          `json:"hasDefault"`
	Options     []string      `json:"options,omitempty"`
}

// PreparedScript combines parsed metadata with the rendered source so callers
// can populate a form and prepare execution through one API.
type PreparedScript struct {
	Source   string    `json:"source"`
	Metadata *Metadata `json:"metadata,omitempty"`
}

type rawMetadata struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  []rawParameter `json:"parameters"`
}

type rawParameter struct {
	Name        string          `json:"name"`
	Label       string          `json:"label"`
	Description string          `json:"description"`
	Placeholder string          `json:"placeholder"`
	Type        ParameterType   `json:"type"`
	Required    bool            `json:"required"`
	Default     json.RawMessage `json:"default"`
	Options     []string        `json:"options"`
}

// Parse extracts and validates @frida-gui metadata. It returns (nil, nil) when
// the source has no metadata, allowing existing scripts to keep working.
func Parse(source string) (*Metadata, error) {
	comments := extractComments(source)
	var payload string
	for _, comment := range comments {
		candidate, found, err := metadataJSON(comment)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		if payload != "" {
			return nil, errors.New("脚本中只能声明一个 @frida-gui 元数据块")
		}
		payload = candidate
	}
	if payload == "" {
		return nil, nil
	}

	var raw rawMetadata
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("解析 @frida-gui 元数据失败: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, fmt.Errorf("解析 @frida-gui 元数据失败: %w", err)
	}

	metadata := &Metadata{
		Name:        strings.TrimSpace(raw.Name),
		Description: strings.TrimSpace(raw.Description),
		Parameters:  make([]Parameter, 0, len(raw.Parameters)),
	}
	seen := make(map[string]struct{}, len(raw.Parameters))
	for index, item := range raw.Parameters {
		parameter, err := normalizeParameter(item)
		if err != nil {
			return nil, fmt.Errorf("参数 #%d: %w", index+1, err)
		}
		if _, exists := seen[parameter.Name]; exists {
			return nil, fmt.Errorf("参数 #%d: 参数名重复: %s", index+1, parameter.Name)
		}
		seen[parameter.Name] = struct{}{}
		metadata.Parameters = append(metadata.Parameters, parameter)
	}
	return metadata, nil
}

// Render validates values against metadata and replaces {{parameterName}}
// placeholders with JavaScript literals. A script without metadata is returned
// byte-for-byte unchanged, even when values is non-empty.
func Render(source string, values map[string]any) (string, error) {
	metadata, err := Parse(source)
	if err != nil {
		return "", err
	}
	if metadata == nil {
		return source, nil
	}
	return renderWithMetadata(source, metadata, values)
}

// Prepare parses metadata and renders source in a single operation.
func Prepare(source string, values map[string]any) (PreparedScript, error) {
	metadata, err := Parse(source)
	if err != nil {
		return PreparedScript{}, err
	}
	if metadata == nil {
		return PreparedScript{Source: source}, nil
	}
	rendered, err := renderWithMetadata(source, metadata, values)
	if err != nil {
		return PreparedScript{}, err
	}
	return PreparedScript{Source: rendered, Metadata: metadata}, nil
}

func renderWithMetadata(source string, metadata *Metadata, values map[string]any) (string, error) {
	resolved, err := resolveValues(metadata, values)
	if err != nil {
		return "", err
	}
	var renderErr error
	rendered := placeholderPattern.ReplaceAllStringFunc(source, func(placeholder string) string {
		if renderErr != nil {
			return placeholder
		}
		match := placeholderPattern.FindStringSubmatch(placeholder)
		name := strings.TrimSpace(match[1])
		value, exists := resolved[name]
		if !exists {
			renderErr = fmt.Errorf("脚本使用了未声明的参数占位符: %s", name)
			return placeholder
		}
		literal, literalErr := javascriptLiteral(value)
		if literalErr != nil {
			renderErr = fmt.Errorf("渲染参数 %s 失败: %w", name, literalErr)
			return placeholder
		}
		return literal
	})
	if renderErr != nil {
		return "", renderErr
	}
	return rendered, nil
}

func normalizeParameter(raw rawParameter) (Parameter, error) {
	parameter := Parameter{
		Name:        strings.TrimSpace(raw.Name),
		Label:       strings.TrimSpace(raw.Label),
		Description: strings.TrimSpace(raw.Description),
		Placeholder: strings.TrimSpace(raw.Placeholder),
		Type:        raw.Type,
		Required:    raw.Required,
		Options:     append([]string(nil), raw.Options...),
	}
	if !parameterNamePattern.MatchString(parameter.Name) {
		return Parameter{}, fmt.Errorf("参数名 %q 无效，只能使用字母、数字、下划线和连字符，且不能以数字开头", parameter.Name)
	}
	if parameter.Label == "" {
		parameter.Label = parameter.Name
	}
	switch parameter.Type {
	case TypeString, TypeNumber, TypeBoolean:
		if len(parameter.Options) != 0 {
			return Parameter{}, fmt.Errorf("%s 类型不能声明 options", parameter.Type)
		}
	case TypeSelect:
		if len(parameter.Options) == 0 {
			return Parameter{}, errors.New("select 类型必须至少声明一个 option")
		}
		seen := make(map[string]struct{}, len(parameter.Options))
		for _, option := range parameter.Options {
			if _, exists := seen[option]; exists {
				return Parameter{}, fmt.Errorf("select option 重复: %q", option)
			}
			seen[option] = struct{}{}
		}
	default:
		return Parameter{}, fmt.Errorf("不支持的参数类型: %q", parameter.Type)
	}

	if len(raw.Default) != 0 {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(raw.Default))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return Parameter{}, fmt.Errorf("default 无效: %w", err)
		}
		if value == nil {
			return Parameter{}, errors.New("default 不能为 null")
		}
		if err := validateValue(parameter, value); err != nil {
			return Parameter{}, fmt.Errorf("default 无效: %w", err)
		}
		parameter.Default = value
		parameter.HasDefault = true
	}
	return parameter, nil
}

func resolveValues(metadata *Metadata, supplied map[string]any) (map[string]any, error) {
	parameters := make(map[string]Parameter, len(metadata.Parameters))
	for _, parameter := range metadata.Parameters {
		parameters[parameter.Name] = parameter
	}

	unknown := make([]string, 0)
	for name := range supplied {
		if _, exists := parameters[name]; !exists {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) != 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("提供了未声明的脚本参数: %s", strings.Join(unknown, ", "))
	}

	resolved := make(map[string]any, len(metadata.Parameters))
	for _, parameter := range metadata.Parameters {
		value, suppliedValue := supplied[parameter.Name]
		if !suppliedValue {
			if parameter.HasDefault {
				value = parameter.Default
			} else if parameter.Required {
				return nil, fmt.Errorf("缺少必填脚本参数: %s", parameter.Name)
			} else {
				value = undefinedValue{}
			}
		}
		if _, undefined := value.(undefinedValue); !undefined {
			if err := validateValue(parameter, value); err != nil {
				return nil, fmt.Errorf("脚本参数 %s 无效: %w", parameter.Name, err)
			}
		}
		resolved[parameter.Name] = value
	}
	return resolved, nil
}

func validateValue(parameter Parameter, value any) error {
	switch parameter.Type {
	case TypeString:
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("需要 string，收到 %s", valueType(value))
		}
		if parameter.Required && strings.TrimSpace(text) == "" {
			return errors.New("必填字符串不能为空")
		}
	case TypeNumber:
		if !isFiniteNumber(value) {
			return fmt.Errorf("需要有限 number，收到 %s", valueType(value))
		}
	case TypeBoolean:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("需要 boolean，收到 %s", valueType(value))
		}
	case TypeSelect:
		selected, ok := value.(string)
		if !ok {
			return fmt.Errorf("需要 select 字符串，收到 %s", valueType(value))
		}
		for _, option := range parameter.Options {
			if selected == option {
				return nil
			}
		}
		return fmt.Errorf("%q 不在允许的 options 中", selected)
	default:
		return fmt.Errorf("不支持的参数类型: %q", parameter.Type)
	}
	return nil
}

func isFiniteNumber(value any) bool {
	switch number := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return !math.IsNaN(float64(number)) && !math.IsInf(float64(number), 0)
	case float64:
		return !math.IsNaN(number) && !math.IsInf(number, 0)
	case json.Number:
		parsed, err := strconv.ParseFloat(string(number), 64)
		return err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	default:
		return false
	}
}

type undefinedValue struct{}

func javascriptLiteral(value any) (string, error) {
	if _, ok := value.(undefinedValue); ok {
		return "undefined", nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func valueType(value any) string {
	if value == nil {
		return "null"
	}
	return reflect.TypeOf(value).String()
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("元数据 JSON 后存在额外内容")
	}
	return err
}

func metadataJSON(comment string) (string, bool, error) {
	comment = normalizeComment(comment)
	markerIndex := strings.Index(comment, metadataMarker)
	if markerIndex < 0 {
		return "", false, nil
	}
	afterMarker := comment[markerIndex+len(metadataMarker):]
	if afterMarker != "" {
		first := afterMarker[0]
		if first != '{' && first != '\r' && first != '\n' && first != ' ' && first != '\t' {
			return "", false, nil
		}
	}
	payload := strings.TrimSpace(afterMarker)
	if payload == "" || payload[0] != '{' {
		return "", true, errors.New("@frida-gui 标记后缺少 JSON 对象")
	}
	object, rest, err := extractJSONObject(payload)
	if err != nil {
		return "", true, fmt.Errorf("读取 @frida-gui JSON 失败: %w", err)
	}
	rest = strings.TrimSpace(rest)
	if rest != "" && rest != "@end-frida-gui" {
		return "", true, errors.New("@frida-gui JSON 后只能跟随可选的 @end-frida-gui 标记")
	}
	return object, true, nil
}

func extractJSONObject(payload string) (string, string, error) {
	depth := 0
	inString := false
	escaped := false
	for index, char := range payload {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == '"' {
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end := index + 1
				return payload[:end], payload[end:], nil
			}
			if depth < 0 {
				return "", "", errors.New("JSON 对象括号不匹配")
			}
		}
	}
	return "", "", errors.New("JSON 对象未闭合")
}

func normalizeComment(comment string) string {
	lines := strings.Split(strings.ReplaceAll(comment, "\r\n", "\n"), "\n")
	for index, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "*") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		}
		lines[index] = line
	}
	return strings.Join(lines, "\n")
}

func extractComments(source string) []string {
	comments := make([]string, 0)
	for index := 0; index < len(source); {
		switch source[index] {
		case '\'', '"', '`':
			index = skipQuoted(source, index, source[index])
		case '/':
			if index+1 >= len(source) {
				index++
				continue
			}
			switch source[index+1] {
			case '*':
				end := strings.Index(source[index+2:], "*/")
				if end < 0 {
					comments = append(comments, source[index+2:])
					return comments
				}
				end += index + 2
				comments = append(comments, source[index+2:end])
				index = end + 2
			case '/':
				var lines []string
				for {
					lineStart := index + 2
					lineEnd := strings.IndexByte(source[lineStart:], '\n')
					if lineEnd < 0 {
						lines = append(lines, strings.TrimSuffix(source[lineStart:], "\r"))
						index = len(source)
						break
					}
					lineEnd += lineStart
					lines = append(lines, strings.TrimSuffix(source[lineStart:lineEnd], "\r"))
					next := lineEnd + 1
					for next < len(source) && (source[next] == ' ' || source[next] == '\t') {
						next++
					}
					if next+1 < len(source) && source[next] == '/' && source[next+1] == '/' {
						index = next
						continue
					}
					index = lineEnd + 1
					break
				}
				comments = append(comments, strings.Join(lines, "\n"))
			default:
				index++
			}
		default:
			index++
		}
	}
	return comments
}

func skipQuoted(source string, start int, quote byte) int {
	escaped := false
	for index := start + 1; index < len(source); index++ {
		if escaped {
			escaped = false
			continue
		}
		if source[index] == '\\' {
			escaped = true
			continue
		}
		if source[index] == quote {
			return index + 1
		}
		if quote != '`' && (source[index] == '\r' || source[index] == '\n') {
			return index + 1
		}
	}
	return len(source)
}
