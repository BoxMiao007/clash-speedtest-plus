package filter_test

import (
	"strings"
	"testing"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/filter"
)

func mustCompile(t *testing.T, spec filter.Spec) *filter.Matcher {
	t.Helper()
	matcher, err := filter.Compile(spec)
	if err != nil {
		t.Fatalf("Compile(%+v) 返回错误：%v", spec, err)
	}
	return matcher
}

func assertMatch(t *testing.T, matcher *filter.Matcher, node string, want bool) {
	t.Helper()
	if got := matcher.Match(node); got != want {
		t.Errorf("Match(%q) = %v，期望 %v", node, got, want)
	}
}

func TestNormalize(t *testing.T) {
	// 空模式补 simplified、空组合补 and、表达式去首尾空白。
	cases := []struct {
		in   filter.Spec
		want filter.Spec
	}{
		{filter.Spec{}, filter.Spec{Mode: filter.ModeSimple, Combine: filter.CombineAnd}},
		{
			filter.Spec{Expression: "  香港 !试用  "},
			filter.Spec{Mode: filter.ModeSimple, Expression: "香港 !试用", Combine: filter.CombineAnd},
		},
		{
			filter.Spec{Mode: filter.ModeAdvanced, Expression: " ^HK ", Combine: filter.CombineOr},
			filter.Spec{Mode: filter.ModeAdvanced, Expression: "^HK", Combine: filter.CombineOr},
		},
	}
	for _, c := range cases {
		if got := filter.Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%+v) = %+v，期望 %+v", c.in, got, c.want)
		}
	}
}

func TestEmptyExpressionMatchesAll(t *testing.T) {
	// 两种模式的空表达式都匹配全部，调用方不需要区分是否为 nil。
	specs := []filter.Spec{
		{},
		{Expression: "   "},
		{Mode: filter.ModeSimple},
		{Mode: filter.ModeAdvanced},
		{Mode: filter.ModeAdvanced, Expression: "\n\t "},
	}
	for _, spec := range specs {
		matcher, err := filter.Compile(spec)
		if err != nil {
			t.Fatalf("Compile(%+v) 返回错误：%v", spec, err)
		}
		assertMatch(t, matcher, "任意节点", true)
		assertMatch(t, matcher, "", true)
	}
}

func TestNilMatcherMatchesAll(t *testing.T) {
	// 契约：nil matcher 视为匹配全部。
	var matcher *filter.Matcher
	assertMatch(t, matcher, "香港 01", true)
	assertMatch(t, matcher, "", true)
}

func TestSimpleAnd(t *testing.T) {
	matcher := mustCompile(t, filter.Spec{Mode: filter.ModeSimple, Expression: "香港 *IPLC"})
	cases := []struct {
		node string
		want bool
	}{
		{"香港 01 IPLC", true},
		{"香港 02 IPLC 试用", true},
		{"香港 01", false},
		{"日本 01 IPLC", false},
		{"", false},
	}
	for _, c := range cases {
		assertMatch(t, matcher, c.node, c.want)
	}
}

func TestSimpleOr(t *testing.T) {
	// Mode 留空走 Normalize 的默认简化模式。
	matcher := mustCompile(t, filter.Spec{Expression: "香港 日本", Combine: filter.CombineOr})
	cases := []struct {
		node string
		want bool
	}{
		{"香港 01", true},
		{"日本 02", true},
		{"新加坡 01", false},
	}
	for _, c := range cases {
		assertMatch(t, matcher, c.node, c.want)
	}

	// 同一表达式切到「与」后语义变化：必须同时包含两个词。
	andMatcher := mustCompile(t, filter.Spec{Expression: "香港 日本", Combine: filter.CombineAnd})
	assertMatch(t, andMatcher, "香港 01", false)
	assertMatch(t, andMatcher, "香港 日本 中转", true)
}

func TestSimpleExclude(t *testing.T) {
	matcher := mustCompile(t, filter.Spec{Expression: "香港 !试用"})
	assertMatch(t, matcher, "香港 01", true)
	assertMatch(t, matcher, "香港 02 试用", false)
	assertMatch(t, matcher, "日本 01", false)

	// 只有排除词时，除命中排除词的节点外全部保留。
	onlyExclude := mustCompile(t, filter.Spec{Expression: "!试用"})
	assertMatch(t, onlyExclude, "香港 01", true)
	assertMatch(t, onlyExclude, "香港 02 试用", false)
	assertMatch(t, onlyExclude, "日本 01", true)

	// 排除词里的通配同样生效。
	wildcardExclude := mustCompile(t, filter.Spec{Expression: "!*试用"})
	assertMatch(t, wildcardExclude, "香港 02 试用", false)
	assertMatch(t, wildcardExclude, "香港 02", true)
}

func TestSimpleWildcard(t *testing.T) {
	// 词按「包含」语义匹配，因此 HK-012 里含子串 HK-01，算命中。
	matcher := mustCompile(t, filter.Spec{Expression: "HK-??"})
	cases := []struct {
		node string
		want bool
	}{
		{"HK-01", true},
		{"HK-1", false},
		{"HK-012", true},
		{"前缀 HK-99 后缀", true},
	}
	for _, c := range cases {
		assertMatch(t, matcher, c.node, c.want)
	}

	middle := mustCompile(t, filter.Spec{Expression: "HK-*-IPLC"})
	assertMatch(t, middle, "HK-01-IPLC", true)
	assertMatch(t, middle, "HK-IPLC", false)
	assertMatch(t, middle, "HK-01", false)
}

func TestSimpleUnicodeWildcard(t *testing.T) {
	// ? 按 rune 计数：一个汉字算一个字符，不是按字节；额外一个 ? 就要求再多一个字符。
	matcher := mustCompile(t, filter.Spec{Expression: "香?"})
	assertMatch(t, matcher, "香港", true)
	assertMatch(t, matcher, "香", false)

	matcher = mustCompile(t, filter.Spec{Expression: "香??"})
	assertMatch(t, matcher, "香港", false)

	matcher = mustCompile(t, filter.Spec{Expression: "北京?"})
	assertMatch(t, matcher, "北京市", true)
	assertMatch(t, matcher, "北京", false)
}

func TestSimpleCaseInsensitive(t *testing.T) {
	matcher := mustCompile(t, filter.Spec{Expression: "hk-01"})
	assertMatch(t, matcher, "HK-01", true)
	assertMatch(t, matcher, "Hk-01 香港", true)

	// Unicode 简单折叠同样生效。
	accented := mustCompile(t, filter.Spec{Expression: "É"})
	assertMatch(t, accented, "节点 é", true)
	assertMatch(t, accented, "节点 e", false)
}

func TestSimpleWhitespaceTokens(t *testing.T) {
	matcher := mustCompile(t, filter.Spec{Expression: "  \t香港   *IPLC \n !试用 "})
	assertMatch(t, matcher, "香港 01 IPLC", true)
	assertMatch(t, matcher, "香港 02 IPLC 试用", false)
}

func TestSimpleInvalidExclude(t *testing.T) {
	for _, expression := range []string{"!", "   !   ", "香港 !"} {
		if _, err := filter.Compile(filter.Spec{Expression: expression}); err == nil {
			t.Errorf("Compile(%q) 期望返回错误，实际为 nil", expression)
		}
	}
}

func TestAdvancedRegex(t *testing.T) {
	matcher := mustCompile(t, filter.Spec{Mode: filter.ModeAdvanced, Expression: `^HK-\d+$`})
	cases := []struct {
		node string
		want bool
	}{
		{"HK-01", true},
		{"HK-1", true},
		{"HK-", false},
		{"HK-01 香港", false},
		{"前缀 HK-01", false},
	}
	for _, c := range cases {
		assertMatch(t, matcher, c.node, c.want)
	}
}

func TestAdvancedCaseSensitive(t *testing.T) {
	matcher := mustCompile(t, filter.Spec{Mode: filter.ModeAdvanced, Expression: `^hk`})
	assertMatch(t, matcher, "hk-01", true)
	assertMatch(t, matcher, "HK-01", false)
}

func TestAdvancedInvalidRegex(t *testing.T) {
	for _, expression := range []string{"(", "*", "[a-"} {
		_, err := filter.Compile(filter.Spec{Mode: filter.ModeAdvanced, Expression: expression})
		if err == nil {
			t.Errorf("Compile(%q) 期望返回错误，实际为 nil", expression)
			continue
		}
		if !strings.Contains(err.Error(), "正则") {
			t.Errorf("Compile(%q) 的错误信息 %q 未提示正则无效", expression, err.Error())
		}
	}
}

func TestUnknownModeAndCombine(t *testing.T) {
	if _, err := filter.Compile(filter.Spec{Mode: filter.Mode("regex"), Expression: "香港"}); err == nil {
		t.Error("未知过滤模式期望返回错误，实际为 nil")
	}
	if _, err := filter.Compile(filter.Spec{Expression: "香港", Combine: filter.Combine("xor")}); err == nil {
		t.Error("未知组合方式期望返回错误，实际为 nil")
	}
}

// TestMatchAllocations 用 testing.AllocsPerRun 实测 Match 的分配次数。
// 简化模式要求手写通配匹配、每次调用零分配；正则模式的分配数字只报告，不做断言。
func TestMatchAllocations(t *testing.T) {
	simple := mustCompile(t, filter.Spec{Expression: "香港 *IPLC !试用"})
	sink := false
	allocs := testing.AllocsPerRun(2000, func() {
		sink = simple.Match("香港 01 IPLC 中转")
	})
	if !sink {
		t.Fatal("简化模式 Match 未命中预期节点")
	}
	t.Logf("简化模式 Match 每次分配：%.2f 次", allocs)
	if allocs != 0 {
		t.Errorf("简化模式 Match 每次分配 %.2f 次，期望 0 次（通配匹配不应编译正则或拼接字符串）", allocs)
	}

	advanced := mustCompile(t, filter.Spec{Mode: filter.ModeAdvanced, Expression: `^HK-\d+$`})
	advancedAllocs := testing.AllocsPerRun(2000, func() {
		sink = advanced.Match("HK-01")
	})
	if !sink {
		t.Fatal("正则模式 Match 未命中预期节点")
	}
	t.Logf("正则模式 Match 每次分配：%.2f 次（仅报告，未做零分配断言）", advancedAllocs)
}
