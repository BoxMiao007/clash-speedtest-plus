// Package filter 实现节点名过滤：简化关键词模式与完整正则模式。
//
// 简化模式的每个词按「包含」语义匹配节点名，等价于在词的两端各补一个 *；
// 通配匹配由本包手写完成，不编译正则、不拼接字符串，每次 Match 不分配内存。
package filter

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Mode 是过滤模式。
type Mode string

const (
	// ModeSimple 是简化关键词模式（默认）。
	ModeSimple Mode = "simple"
	// ModeAdvanced 是完整 Go 正则模式。
	ModeAdvanced Mode = "advanced"
)

// Combine 是简化模式下正向词之间的组合方式。排除词永远是独立的否定条件。
type Combine string

const (
	// CombineAnd 表示正向词必须全部命中。
	CombineAnd Combine = "and"
	// CombineOr 表示正向词任一命中即可。
	CombineOr Combine = "or"
)

// Spec 是过滤条件的原始输入，通常直接来自界面或预设。
type Spec struct {
	Mode       Mode
	Expression string
	Combine    Combine
}

// token 是简化模式解析出的一个词；exclude 表示 ! 前缀的排除词。
type token struct {
	pattern string
	exclude bool
}

// Matcher 是编译后的过滤器。编译完成后只读，可被多个 goroutine 同时使用。
type Matcher struct {
	mode    Mode
	combine Combine
	tokens  []token
	re      *regexp.Regexp
}

// Normalize 把空的模式补成 simple、空的组合方式补成 and，并去掉表达式首尾空白。
// 其它值原样保留，交给 Compile 报错，避免界面层静默改写用户输入。
func Normalize(spec Spec) Spec {
	normalized := Spec{
		Mode:       spec.Mode,
		Expression: strings.TrimSpace(spec.Expression),
		Combine:    spec.Combine,
	}
	if normalized.Mode == "" {
		normalized.Mode = ModeSimple
	}
	if normalized.Combine == "" {
		normalized.Combine = CombineAnd
	}
	return normalized
}

// Compile 校验并编译过滤条件。空表达式返回 (nil, nil)，nil Matcher 匹配全部；
// 非法表达式（高级模式的坏正则、简化模式只有「!」的词等）返回中文错误信息。
func Compile(spec Spec) (*Matcher, error) {
	spec = Normalize(spec)
	if spec.Expression == "" {
		return nil, nil
	}
	switch spec.Mode {
	case ModeSimple:
		return compileSimple(spec)
	case ModeAdvanced:
		return compileAdvanced(spec)
	default:
		return nil, fmt.Errorf("未知的过滤模式：%q", string(spec.Mode))
	}
}

func compileAdvanced(spec Spec) (*Matcher, error) {
	re, err := regexp.Compile(spec.Expression)
	if err != nil {
		return nil, fmt.Errorf("正则表达式无效：%w", err)
	}
	return &Matcher{mode: ModeAdvanced, re: re}, nil
}

func compileSimple(spec Spec) (*Matcher, error) {
	switch spec.Combine {
	case CombineAnd, CombineOr:
	default:
		return nil, fmt.Errorf("未知的组合方式：%q", string(spec.Combine))
	}
	fields := strings.Fields(spec.Expression)
	if len(fields) == 0 {
		return nil, nil
	}
	matcher := &Matcher{mode: ModeSimple, combine: spec.Combine, tokens: make([]token, 0, len(fields))}
	for _, field := range fields {
		item := token{pattern: field}
		if strings.HasPrefix(field, "!") {
			item.exclude = true
			item.pattern = field[len("!"):]
			if item.pattern == "" {
				return nil, errors.New("排除词不能只有「!」")
			}
		}
		matcher.tokens = append(matcher.tokens, item)
	}
	return matcher, nil
}

// Match 判断节点名是否通过过滤。nil Matcher 视为匹配全部。
func (m *Matcher) Match(name string) bool {
	if m == nil {
		return true
	}
	if m.mode == ModeAdvanced {
		return m.re.MatchString(name)
	}
	// 排除词先判：命中任一排除词就整体不匹配，与正向词的「与/或」无关。
	for i := range m.tokens {
		if m.tokens[i].exclude && matchContains(m.tokens[i].pattern, name) {
			return false
		}
	}
	hasPositive := false
	allMatched := true
	anyMatched := false
	for i := range m.tokens {
		if m.tokens[i].exclude {
			continue
		}
		hasPositive = true
		if matchContains(m.tokens[i].pattern, name) {
			anyMatched = true
		} else {
			allMatched = false
		}
	}
	if !hasPositive {
		return true
	}
	if m.combine == CombineOr {
		return anyMatched
	}
	return allMatched
}

// matchContains 判断 pattern 是否命中 text 的某个子串，等价于在 pattern 两端
// 各补一个 *；这里不拼接字符串，避免每次匹配分配内存。
func matchContains(pattern, text string) bool {
	if pattern == "" {
		return true
	}
	if matchWildcardPrefix(pattern, text) {
		return true
	}
	for index := 0; index < len(text); {
		_, size := utf8.DecodeRuneInString(text[index:])
		index += size
		if matchWildcardPrefix(pattern, text[index:]) {
			return true
		}
	}
	return false
}

// matchWildcardPrefix 判断 pattern 能否完整匹配 text 的某个前缀。
// * 匹配任意长度（含空），? 匹配恰好一个 rune，其余字符按 rune 大小写不敏感比较。
// 全程只推进下标，不分配内存，也不编译正则。
func matchWildcardPrefix(pattern, text string) bool {
	var (
		p        int // pattern 当前字节下标
		t        int // text 当前字节下标
		starP    = -1
		starText int
	)
	for {
		if p >= len(pattern) {
			return true
		}
		if pattern[p] == '*' {
			// 记录最近一个 *，先让它匹配空串；后续不匹配时再回溯多吃一个 rune。
			starP = p
			starText = t
			p++
			continue
		}
		if t < len(text) {
			if pattern[p] == '?' {
				_, size := utf8.DecodeRuneInString(text[t:])
				t += size
				p++
				continue
			}
			patternRune, patternSize := utf8.DecodeRuneInString(pattern[p:])
			textRune, textSize := utf8.DecodeRuneInString(text[t:])
			if runeEqualFold(patternRune, textRune) {
				p += patternSize
				t += textSize
				continue
			}
		}
		if starP < 0 || starText >= len(text) {
			return false
		}
		_, size := utf8.DecodeRuneInString(text[starText:])
		starText += size
		t = starText
		p = starP + 1
	}
}

// runeEqualFold 按 Unicode 简单折叠比较两个 rune（与 strings.EqualFold 同一套规则），
// 只做比较，不生成新字符串。
func runeEqualFold(a, b rune) bool {
	if a == b {
		return true
	}
	for folded := unicode.SimpleFold(a); folded != a; folded = unicode.SimpleFold(folded) {
		if folded == b {
			return true
		}
	}
	return false
}
