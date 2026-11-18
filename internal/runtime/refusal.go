package runtime

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/save"
)

// maxRefusalRescues 限制一次普通叙事回合因拒答而自动收束的次数，避免死循环。
const maxRefusalRescues = 1

// refusalMarkers 是模型拒答时常见的开头。只在句首或空白之后匹配，避免把叙事里的
// “她不能相信自己的眼睛”误当成拒答。
var refusalMarkers = []string{
	"抱歉", "对不起", "很抱歉", "非常抱歉",
	"我无法", "我不能", "我没办法",
	"无法继续", "不能继续", "无法提供", "不能提供",
	"无法协助", "不能协助", "无法完成", "不能完成",
	"作为AI", "作为一个AI", "作为人工智能", "作为语言模型",
	"I'm sorry", "I am sorry", "I can't", "I cannot", "I won't", "I will not",
	"I'm unable", "I am unable", "As an AI", "As a language model",
	"I must decline", "I can't continue", "I cannot continue",
}

// absorbRefusal 回滚本次调用已经流出的显示项，只保留拒答之前的完整叙事，
// 并返回可以写入模型历史的助手消息（只含实际保留的部分）。
func (d *draft) absorbRefusal(msg llm.Message, before []save.DisplayItem) llm.Message {
	d.display = slices.Clone(before)
	keep := refusalNarrative(msg.TextOf())
	if keep == "" {
		return llm.Message{Role: llm.RoleAssistant}
	}
	if prev := d.lastText(); prev != "" {
		if n := dedupOverlap(prev, keep); n > 0 {
			keep = keep[n:]
		}
	}
	if keep == "" {
		return llm.Message{Role: llm.RoleAssistant}
	}
	d.appendText(keep)
	return llm.Message{Role: llm.RoleAssistant, Parts: []llm.Part{llm.Text(keep)}}
}

// withoutRefusal 用于选项、核对等非普通叙事调用：拒答没有可保留的续写语义，
// 只留下拒绝语之前的文本，随后交给原有错误路径收尾。
func withoutRefusal(msg llm.Message) llm.Message {
	role := msg.Role
	if role == "" {
		role = llm.RoleAssistant
	}
	keep := refusalNarrative(msg.TextOf())
	if keep == "" {
		return llm.Message{Role: role}
	}
	return llm.Message{Role: role, Parts: []llm.Part{llm.Text(keep)}}
}

// refusalNarrative 返回拒绝语之前的完整叙事前缀。找不到可识别的拒绝语、
// 或前缀只是半句话时返回空串，宁可丢弃也不把拒绝语写进正文。
func refusalNarrative(text string) string {
	cut, ok := refusalCut(text)
	if !ok {
		return ""
	}
	keep := strings.TrimRight(cut, " \t\r\n")
	if keep == "" {
		return ""
	}
	runes := utf8.RuneCountInString(keep)
	if runes < 4 {
		return ""
	}
	last, _ := utf8.DecodeLastRuneInString(keep)
	if runes < 80 && !refusalSentenceEnd(last) {
		return ""
	}
	return keep
}

func refusalCut(text string) (string, bool) {
	lower := strings.ToLower(text)
	best := -1
	for _, marker := range refusalMarkers {
		m := strings.ToLower(marker)
		for from := 0; from <= len(lower); {
			i := strings.Index(lower[from:], m)
			if i < 0 {
				break
			}
			i += from
			if refusalBoundary(text, i) {
				if best < 0 || i < best {
					best = i
				}
				break
			}
			from = i + len(m)
		}
	}
	if best < 0 {
		return "", false
	}
	return text[:best], true
}

func refusalBoundary(text string, i int) bool {
	if i <= 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(text[:i])
	return unicode.IsSpace(r) || strings.ContainsRune("。！？!?…；;", r)
}

func refusalSentenceEnd(r rune) bool {
	switch r {
	case '。', '.', '！', '？', '!', '?', '…', '；', ';', '”', '"', '」', '』':
		return true
	}
	return false
}
