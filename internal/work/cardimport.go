package work

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/wahhheee/aint/internal/pkg/errs"
)

// 角色卡导入：把 SillyTavern 角色卡（PNG 里的 ccv3/chara 文本块、.charx 压缩包里的 card.json、
// 或裸 JSON）解析成结构，再合成为一个 aint 作品目录（FileSet）。
//
// 映射：
//
//	卡名 / 描述          → lore/card.md（load: always）+ work.yaml 的标题与简介
//	性格 / 场景 / 附加要求 → prompts/narrator.md
//	对话样例             → prompts/narrator.md 的语感段
//	first_mes / 备选开场 → prompts/openings/NN.md（static，正文包在 raw 块里）
//	character_book 条目  → lore/worldbook/NNN.md（load: index，按需读取）

// STCard 是解析后的角色卡。
type STCard struct {
	Name             string
	Creator          string
	CharacterVersion string
	Description      string
	Personality      string
	Scenario         string
	FirstMes         string
	MesExample       string
	SystemPrompt     string
	PostHistory      string
	CreatorNotes     string
	AlternateGreets  []string
	Tags             []string
	Book             []STBookEntry
}

// STBookEntry 是世界书条目。
type STBookEntry struct {
	Keys     []string
	Content  string
	Comment  string
	Name     string
	Enabled  bool
	Constant bool
	Order    int
}

// ErrNotCard 表示上传的内容不是角色卡。
var ErrNotCard = errors.New("不是角色卡")

const maxCardBytes = 64 << 20

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

// ParseSTCard 解析角色卡：PNG（ccv3/chara）、.charx（zip 内 card.json）或裸 JSON。
func ParseSTCard(data []byte) (*STCard, error) {
	if len(data) > maxCardBytes {
		return nil, rejectf("角色卡文件过大（上限 %d MiB）", maxCardBytes>>20)
	}
	var raw []byte
	switch {
	case bytes.HasPrefix(data, pngSignature):
		text, err := pngTextBlocks(data)
		if err != nil {
			return nil, err
		}
		for _, k := range []string{"ccv3", "chara"} {
			v, ok := text[k]
			if !ok {
				continue
			}
			b, err := decodeBase64Loose(v)
			if err != nil {
				return nil, rejectf("角色卡的 %s 字段不是有效的 base64", k)
			}
			raw = b
			break
		}
		if raw == nil {
			return nil, rejectf("%v：PNG 里没有 ccv3/chara 文本块", ErrNotCard)
		}
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		b, err := charxCardJSON(data)
		if err != nil {
			return nil, err
		}
		raw = b
	default:
		trimmed := bytes.TrimSpace(data)
		if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
			return nil, rejectf("可导入的是 PNG、.charx 或 .json 角色卡")
		}
		raw = trimmed
	}
	return normalizeCard(raw)
}

// pngTextBlocks 读取 PNG 的 tEXt / zTXt / iTXt 文本块（keyword → 文本）。
func pngTextBlocks(data []byte) (map[string]string, error) {
	out := map[string]string{}
	off := len(pngSignature)
	for off+8 <= len(data) {
		n := int(binary.BigEndian.Uint32(data[off:]))
		typ := string(data[off+4 : off+8])
		if n < 0 || off+12+n > len(data) {
			return nil, rejectf("角色卡 PNG 数据损坏：文本块长度越界")
		}
		body := data[off+8 : off+8+n]
		switch typ {
		case "tEXt":
			if k, v, ok := splitNul(body); ok {
				out[k] = string(v)
			}
		case "zTXt":
			if k, rest, ok := splitNul(body); ok && len(rest) > 1 && rest[0] == 0 {
				if d, err := inflate(rest[1:]); err == nil {
					out[k] = string(d)
				}
			}
		case "iTXt":
			if k, rest, ok := splitNul(body); ok && len(rest) >= 2 {
				compressed := rest[0] == 1
				rest = rest[2:]
				if _, r2, ok := splitNul(rest); ok {
					if _, r3, ok := splitNul(r2); ok {
						text := r3
						if compressed {
							d, err := inflate(text)
							if err != nil {
								continue
							}
							text = d
						}
						out[k] = string(text)
					}
				}
			}
		case "IEND":
			return out, nil
		}
		off += 12 + n
	}
	return out, nil
}

func splitNul(b []byte) (string, []byte, bool) {
	i := bytes.IndexByte(b, 0)
	if i < 0 {
		return "", nil, false
	}
	return string(b[:i]), b[i+1:], true
}

func inflate(b []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return io.ReadAll(io.LimitReader(r, maxCardBytes))
}

// decodeBase64Loose 兼容标准 / URL-safe / 缺补位的 base64。
func decodeBase64Loose(s string) ([]byte, error) {
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\n', '\r', '\t':
			return -1
		}
		return r
	}, s)
	if len(s)%4 != 0 {
		s += strings.Repeat("=", 4-len(s)%4)
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("base64 解码失败")
}

// charxCardJSON 从 .charx（ZIP）里取出 card.json。
func charxCardJSON(data []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, rejectf("读取 .charx 失败：%v", err)
	}
	var best *zip.File
	for _, f := range zr.File {
		name := strings.ToLower(strings.ReplaceAll(f.Name, `\`, "/"))
		if strings.HasSuffix(name, "card.json") {
			if best == nil || len(f.Name) < len(best.Name) {
				best = f
			}
		}
	}
	if best == nil {
		return nil, rejectf("%v：.charx 里没有 card.json", ErrNotCard)
	}
	rc, err := best.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(io.LimitReader(rc, maxCardBytes))
}

type cardBookEntry struct {
	Keys     []string `json:"keys"`
	Content  string   `json:"content"`
	Comment  string   `json:"comment"`
	Name     string   `json:"name"`
	Enabled  *bool    `json:"enabled"`
	Constant bool     `json:"constant"`
	Order    int      `json:"insertion_order"`
}

type cardPayload struct {
	Name             string    `json:"name"`
	Creator          string    `json:"creator"`
	CharacterVersion string    `json:"character_version"`
	Description      string    `json:"description"`
	Personality      string    `json:"personality"`
	Scenario         string    `json:"scenario"`
	FirstMes         string    `json:"first_mes"`
	MesExample       string    `json:"mes_example"`
	SystemPrompt     string    `json:"system_prompt"`
	PostHistory      string    `json:"post_history_instructions"`
	CreatorNotes     string    `json:"creator_notes"`
	Alternates       []string  `json:"alternate_greetings"`
	Tags             []string  `json:"tags"`
	Book             *cardBook `json:"character_book"`
}

type cardBook struct {
	Entries []cardBookEntry `json:"entries"`
}

// normalizeCard 把 V2/V3（data 包裹）与 V1（字段在根上）统一成一个结构。
func normalizeCard(raw []byte) (*STCard, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, rejectf("角色卡 JSON 解析失败：%v", err)
	}
	payload := raw
	if inner, ok := top["data"]; ok {
		if t := bytes.TrimSpace(inner); len(t) > 0 && t[0] == '{' {
			payload = t
		}
	}
	var p cardPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, rejectf("角色卡字段解析失败：%v", err)
	}
	c := &STCard{
		Name: strings.TrimSpace(p.Name), Creator: strings.TrimSpace(p.Creator),
		CharacterVersion: strings.TrimSpace(p.CharacterVersion),
		Description:      strings.TrimSpace(p.Description),
		Personality:      strings.TrimSpace(p.Personality), Scenario: strings.TrimSpace(p.Scenario),
		FirstMes: strings.TrimSpace(p.FirstMes), MesExample: strings.TrimSpace(p.MesExample),
		SystemPrompt: strings.TrimSpace(p.SystemPrompt), PostHistory: strings.TrimSpace(p.PostHistory),
		CreatorNotes: strings.TrimSpace(p.CreatorNotes), Tags: p.Tags,
	}
	for _, g := range p.Alternates {
		if g = strings.TrimSpace(g); g != "" {
			c.AlternateGreets = append(c.AlternateGreets, g)
		}
	}
	c.Description = cleanText(convertCardTags(c.Description))
	c.Personality = cleanText(c.Personality)
	c.Scenario = cleanText(convertCardTags(c.Scenario))
	c.FirstMes = cleanText(convertCardTags(c.FirstMes))
	c.MesExample = cleanText(c.MesExample)
	c.SystemPrompt = cleanText(c.SystemPrompt)
	c.PostHistory = cleanText(c.PostHistory)
	c.CreatorNotes = cleanText(c.CreatorNotes)
	for i := range c.AlternateGreets {
		c.AlternateGreets[i] = cleanText(convertCardTags(c.AlternateGreets[i]))
	}
	if p.Book != nil {
		for _, e := range p.Book.Entries {
			enabled := true
			if e.Enabled != nil {
				enabled = *e.Enabled
			}
			if !enabled || strings.TrimSpace(e.Content) == "" {
				continue
			}
			c.Book = append(c.Book, STBookEntry{
				Keys: e.Keys, Content: cleanText(e.Content), Comment: cleanText(e.Comment),
				Name: strings.TrimSpace(e.Name), Enabled: enabled, Constant: e.Constant, Order: e.Order,
			})
		}
		sort.SliceStable(c.Book, func(i, j int) bool { return c.Book[i].Order < c.Book[j].Order })
	}
	if c.Name == "" {
		c.Name = "未命名角色卡"
	}
	if c.Description == "" && c.Personality == "" && c.FirstMes == "" {
		return nil, rejectf("%v：卡里没有描述、性格或开场白", ErrNotCard)
	}
	return c, nil
}

// identity 是卡的身份：同名不同作者、或同名但角色定义不同的卡视为不同作品，
// 只改开场白 / 对话样例的再导入仍归入同一作品（作为新版本）。
func (c *STCard) identity() string {
	if c.Creator != "" {
		return c.Name + "\x00" + c.Creator
	}
	// 没有作者名时，用角色定义做指纹，避免同名卡互相覆盖。
	def := strings.Join([]string{c.Description, c.Personality, c.Scenario, c.SystemPrompt}, "\x00")
	sum := sha256.Sum256([]byte(def))
	return c.Name + "\x00" + hex.EncodeToString(sum[:])[:16]
}

// WorkID 给出稳定的作品 ID：身份哈希，同一张卡重复导入不会产生两个作品，同名不同卡也不会冲突。
func (c *STCard) WorkID() string {
	sum := sha256.Sum256([]byte(c.identity()))
	return "card." + hex.EncodeToString(sum[:])[:10]
}

// semverVersion 取卡里的 character_version（规范化为 semver）；没有或无法识别时回退 1.0.0。
func (c *STCard) semverVersion() string {
	v := strings.TrimSpace(c.CharacterVersion)
	if reSemver.MatchString(v) {
		return v
	}
	// 作者常写 1、1.2、v1.2 这类版本号，补成 semver。
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	if v == "" {
		return "1.0.0"
	}
	core, suffix := v, ""
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		core, suffix = v[:i], v[i:]
	}
	parts := strings.Split(core, ".")
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	for i, p := range parts {
		if p == "" {
			parts[i] = "0"
			continue
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return "1.0.0"
			}
		}
	}
	candidate := strings.Join(parts[:3], ".") + suffix
	if reSemver.MatchString(candidate) {
		return candidate
	}
	return "1.0.0"
}

// FileSet 把角色卡合成为一个 aint 作品目录。
func (c *STCard) FileSet() (*FileSet, error) {
	set := NewFileSet()
	add := func(p, body string) error { return set.Add(p, []byte(body)) }

	openings := c.openings()
	var yml strings.Builder
	yml.WriteString("format: 1\n")
	fmt.Fprintf(&yml, "id: %s\n", c.WorkID())
	fmt.Fprintf(&yml, "version: %s\n", c.semverVersion())
	fmt.Fprintf(&yml, "title: %s\n", yamlString(c.Name))
	yml.WriteString("engine: \">=2.0 <3.0\"\n")
	fmt.Fprintf(&yml, "description: %s\n", yamlString(c.summary()))
	yml.WriteString("language: zh-CN\ncontent_rating: mature\n")
	yml.WriteString("tags:\n")
	for _, t := range append(append([]string{}, c.Tags...), "角色卡") {
		if t = strings.TrimSpace(t); t != "" {
			fmt.Fprintf(&yml, "  - %s\n", yamlString(t))
		}
	}
	yml.WriteString("\nnarrator:\n  instructions: prompts/narrator.md\n")
	yml.WriteString("\nopenings:\n")
	for _, o := range openings {
		fmt.Fprintf(&yml, "  - {id: %s, title: %s, description: %s, mode: static, file: %s}\n", o.id, yamlString(o.title), yamlString(o.desc), o.file)
	}
	if err := add("work.yaml", yml.String()); err != nil {
		return nil, err
	}
	if err := add("prompts/narrator.md", c.narrator()); err != nil {
		return nil, err
	}
	for _, o := range openings {
		if err := add(o.file, o.body); err != nil {
			return nil, err
		}
	}
	if err := add("lore/card.md", c.characterLore()); err != nil {
		return nil, err
	}
	for i, e := range c.Book {
		p := fmt.Sprintf("lore/worldbook/%03d.md", i+1)
		if err := add(p, c.bookLore(i+1, e)); err != nil {
			return nil, err
		}
	}
	return set, nil
}

type cardOpening struct {
	id, title, desc, file, body string
}

func (c *STCard) openings() []cardOpening {
	var bodies []string
	if c.FirstMes != "" {
		bodies = append(bodies, c.FirstMes)
	}
	bodies = append(bodies, c.AlternateGreets...)
	if len(bodies) == 0 {
		// 卡里没有开场白时给一个静态占位，否则作品能导入却无法开始游戏。
		bodies = append(bodies, c.placeholderOpening())
	}
	var out []cardOpening
	for i, b := range bodies {
		id := "opening"
		title := "开场"
		file := "prompts/openings/01.md"
		if i > 0 {
			id = fmt.Sprintf("opening-%d", i+1)
			title = fmt.Sprintf("另一段开场 %d", i+1)
			file = fmt.Sprintf("prompts/openings/%02d.md", i+1)
		}
		out = append(out, cardOpening{id: id, title: title, desc: openingExcerpt(b), file: file, body: c.openingBody(b)})
	}
	return out
}

// placeholderOpening 在没有开场白时用角色描述的首段代替，保证新存档总能开始。
func (c *STCard) placeholderOpening() string {
	for _, s := range []string{c.Description, c.Scenario, c.Personality, c.SystemPrompt} {
		if p := firstParagraph(s); p != "" {
			return p
		}
	}
	return "故事从你走进这里开始。"
}

// firstParagraph 取第一段正文（跳过空行与单独的标题行）。
func firstParagraph(s string) string {
	var lines []string
	for _, raw := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			if len(lines) > 0 {
				break
			}
			continue
		}
		if len(lines) == 0 && (strings.HasPrefix(line, "#") || strings.HasPrefix(line, "---")) {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// rawLiquid 把卡片正文原样交给模型：内容放进 {% raw %}，并中和正文里可能提前结束 raw 的 {%。
func rawLiquid(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "{%", "{ %")
	return "{% raw %}\n" + strings.TrimSpace(s) + "\n{% endraw %}"
}

// openingBody 把开场白写成静态开场：原样展示，不做 Liquid 渲染。
func (c *STCard) openingBody(s string) string {
	return rawLiquid(c.expandMacros(s)) + "\n"
}

var (
	reCharMacro   = regexp.MustCompile(`(?i)\{\{\s*(?:char|original)\s*\}\}`)
	reUserMacro   = regexp.MustCompile(`(?i)\{\{\s*(?:user|persona)\s*\}\}`)
	reRandomMacro = regexp.MustCompile(`(?i)\{\{\s*(?:pick|random)\s*::\s*([^{}]*)\}\}`)
	reAnyMacro    = regexp.MustCompile(`\{\{[^{}]{0,200}\}\}`)
)

// expandMacros 拍平酒馆宏：常见宏换成实际文本，取值/时间等动态宏与无法识别的宏一并去掉，
// 避免残留的 {{ }} 被当成 Liquid 表达式或被模型原样输出。
func (c *STCard) expandMacros(s string) string {
	if s == "" {
		return s
	}
	s = reCharMacro.ReplaceAllStringFunc(s, func(string) string { return c.Name })
	s = reUserMacro.ReplaceAllStringFunc(s, func(string) string { return "你" })
	s = reRandomMacro.ReplaceAllStringFunc(s, func(m string) string {
		sub := reRandomMacro.FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		return strings.TrimSpace(strings.SplitN(sub[1], "::", 2)[0])
	})
	s = strings.NewReplacer("<BOT>", c.Name, "<USER>", "你", "<bot>", c.Name, "<user>", "你").Replace(s)
	return reAnyMacro.ReplaceAllString(s, "")
}

func (c *STCard) summary() string {
	src := c.Description
	if src == "" {
		src = c.Scenario
	}
	src = firstRunes(c.expandMacros(src), 120)
	if src == "" {
		src = c.Name
	}
	return src
}

// narrator 生成执笔指令。
func (c *STCard) narrator() string {
	var b strings.Builder
	b.WriteString("# 执笔指令\n\n")
	b.WriteString("这是一部以角色互动为核心的成人向互动小说。玩家以第二人称「你」行动；你负责扮演所有 NPC、推进场景，每轮给出可以直接接手的处境。\n\n")
	b.WriteString("## 基本写法\n\n")
	b.WriteString("- 用第二人称写玩家的所见所感，不替玩家决定行动，也不替玩家定义情绪。\n")
	b.WriteString("- 每次回应推进一小段，落在一个未完成的动作或一句没答完的话上。\n")
	b.WriteString("- 人物说话要具体，带各自的立场与口癖；不写旁白式解释，不写「语气里带着」。\n")
	b.WriteString("- 环境、气味、温度、声响随手交代，让场面站得住。不做总结，不升华，不列选项。\n")
	b.WriteString("- 卡里残留的标记只作参考，不得原样输出：不写 XML / HTML 标签（如 <HotelScene>、</C>、<span>），不写「员工|女仆长」这类标签式排版，一律改成连贯的叙事与对白。\n\n")
	fmt.Fprintf(&b, "## 主要出场人物：%s\n\n", c.Name)
	// 卡片正文包进 raw：作者的模板片段不会被 Liquid 求值，也避免宏残留影响渲染。
	if c.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", rawLiquid(c.expandMacros(c.Description)))
	}
	if c.Personality != "" {
		fmt.Fprintf(&b, "### 性格与说话方式\n\n%s\n\n", rawLiquid(c.expandMacros(c.Personality)))
	}
	if c.Scenario != "" {
		fmt.Fprintf(&b, "### 场景与处境\n\n%s\n\n", rawLiquid(c.expandMacros(c.Scenario)))
	}
	if c.SystemPrompt != "" {
		fmt.Fprintf(&b, "### 作者附加要求\n\n%s\n\n", rawLiquid(c.expandMacros(c.SystemPrompt)))
	}
	if c.PostHistory != "" {
		fmt.Fprintf(&b, "### 每轮都要遵守的硬要求\n\n%s\n\n", rawLiquid(c.expandMacros(c.PostHistory)))
	}
	if c.MesExample != "" {
		b.WriteString("### 语感样例\n\n以下是作者给的对话样例，只作语感参照，不要照抄句子。\n\n")
		b.WriteString(rawLiquid(c.expandMacros(c.MesExample)))
		b.WriteString("\n")
	}
	return b.String()
}

func (c *STCard) characterLore() string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("id: card\n")
	fmt.Fprintf(&b, "title: %s\n", yamlString(c.Name))
	b.WriteString("kind: character\n")
	fmt.Fprintf(&b, "summary: %s\n", yamlString(c.summary()))
	b.WriteString("load: always\n")
	b.WriteString("---\n\n")
	if c.Description != "" {
		b.WriteString(c.expandMacros(c.Description))
		b.WriteString("\n")
	}
	if c.Personality != "" {
		fmt.Fprintf(&b, "\n## 性格\n\n%s\n", c.expandMacros(c.Personality))
	}
	if c.Scenario != "" {
		fmt.Fprintf(&b, "\n## 场景\n\n%s\n", c.expandMacros(c.Scenario))
	}
	// 作者的创作说明 / 使用建议并入角色卡设定，避免这部分信息在导入时丢失。
	if c.CreatorNotes != "" {
		fmt.Fprintf(&b, "\n## 作者附注\n\n%s\n", c.expandMacros(c.CreatorNotes))
	}
	return b.String()
}

func (c *STCard) bookLore(idx int, e STBookEntry) string {
	title := c.expandMacros(e.Comment)
	if title == "" {
		title = c.expandMacros(e.Name)
	}
	keys := make([]string, 0, len(e.Keys))
	for _, k := range e.Keys {
		if k = strings.TrimSpace(c.expandMacros(k)); k != "" {
			keys = append(keys, k)
		}
	}
	if title == "" && len(keys) > 0 {
		n := len(keys)
		if n > 3 {
			n = 3
		}
		title = strings.Join(keys[:n], " / ")
	}
	if title == "" {
		title = fmt.Sprintf("设定 %d", idx)
	}
	content := c.expandMacros(e.Content)
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: wb-%03d\n", idx)
	fmt.Fprintf(&b, "title: %s\n", yamlString(title))
	b.WriteString("kind: setting\n")
	if len(keys) > 0 {
		fmt.Fprintf(&b, "aliases: [%s]\n", yamlList(keys))
	}
	fmt.Fprintf(&b, "summary: %s\n", yamlString(firstRunes(content, 90)))
	b.WriteString("load: index\n")
	b.WriteString("---\n\n")
	b.WriteString(content)
	b.WriteString("\n")
	return b.String()
}

// cleanText 把角色卡里的 HTML 与自定义标记拍平：加粗、斜体转成 Markdown，其余标签去掉。
// cardTagPattern 匹配酒馆卡里用来排版状态栏的自定义标记。
// 这些标记按惯例都是大写：区分大小写可以避免把 <p> 等标准 HTML 当成指标。
var cardTagPattern = regexp.MustCompile(`<(/?)(DT|ST|N|A|O|C|H|S|P|D|T|L)>`)

// convertCardTags 把 <N>/<A>/<O>/<C>/<H>/<S>/<P>/<D>/<T>/<DT>/<L> 这套酒馆排版标记
// 转成可读的 markdown：名字加粗、年龄并入名字、职业斜体、衣着引用块，
// 四项指标并成一行，时间/日期/地点并成一行。未识别的标签留给 cleanText 清掉。
func convertCardTags(s string) string {
	if s == "" || !strings.Contains(s, "<") {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	type piece struct {
		tag  string
		text string
	}
	var out []string
	var metrics []string
	var place []string
	flushMetrics := func() {
		if len(metrics) == 0 {
			return
		}
		out = append(out, strings.Join(metrics, "　"))
		metrics = nil
	}
	flushPlace := func() {
		if len(place) == 0 {
			return
		}
		out = append(out, strings.Join(place, "｜"))
		place = nil
	}
	flushAll := func() {
		flushMetrics()
		flushPlace()
	}
	nameLine := -1

	for _, raw := range strings.Split(s, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			flushAll()
			out = append(out, "")
			continue
		}
		marks := cardTagPattern.FindAllStringSubmatchIndex(line, -1)
		if len(marks) == 0 {
			flushAll()
			out = append(out, line)
			nameLine = -1
			continue
		}
		var items []piece
		var buf strings.Builder
		open := ""
		last := 0
		for _, m := range marks {
			seg := line[last:m[0]]
			if open == "" {
				if t := strings.TrimSpace(seg); t != "" {
					items = append(items, piece{text: t})
				}
			} else {
				buf.WriteString(seg)
			}
			closing := m[2] != m[3]
			name := strings.ToUpper(line[m[4]:m[5]])
			if !closing {
				if open == "" {
					open = name
					buf.Reset()
				}
			} else if open != "" {
				items = append(items, piece{tag: open, text: strings.TrimSpace(buf.String())})
				buf.Reset()
				open = ""
			}
			last = m[1]
		}
		if open != "" {
			buf.WriteString(line[last:])
			items = append(items, piece{tag: open, text: strings.TrimSpace(buf.String())})
		} else if t := strings.TrimSpace(line[last:]); t != "" {
			items = append(items, piece{text: t})
		}
		for _, it := range items {
			if it.tag == "" {
				flushAll()
				if it.text != "" {
					out = append(out, it.text)
				}
				nameLine = -1
				continue
			}
			switch it.tag {
			case "N":
				flushAll()
				out = append(out, "**"+it.text+"**")
				nameLine = len(out) - 1
			case "A":
				if nameLine >= 0 && nameLine == len(out)-1 {
					out[nameLine] = strings.TrimSuffix(out[nameLine], "**") + "（" + it.text + "）**"
				} else {
					flushAll()
					out = append(out, "（"+it.text+"）")
					nameLine = -1
				}
			case "O":
				flushAll()
				out = append(out, "*"+it.text+"*")
				nameLine = -1
			case "C":
				flushAll()
				out = append(out, "> "+it.text)
				nameLine = -1
			case "H", "S", "P", "D":
				metrics = append(metrics, it.tag+" "+it.text)
				nameLine = -1
			case "T", "DT", "L":
				place = append(place, it.text)
				nameLine = -1
			case "ST":
				flushAll()
				nameLine = -1
			}
		}
	}
	flushAll()
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func cleanText(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	for _, r := range [][2]string{
		{"<br>", "\n"}, {"<br/>", "\n"}, {"<br />", "\n"}, {"</br>", "\n"},
		{"<p>", "\n"}, {"</p>", "\n\n"},
		{"<strong>", "**"}, {"</strong>", "**"},
		{"<b>", "**"}, {"</b>", "**"},
		{"<em>", "*"}, {"</em>", "*"},
		{"<i>", "*"}, {"</i>", "*"},
		{"<hr>", "\n\n---\n\n"}, {"<hr/>", "\n\n---\n\n"},
		{"&nbsp;", " "}, {"&amp;", "&"}, {"&lt;", "<"}, {"&gt;", ">"},
		{"&quot;", "\""}, {"&#39;", "'"},
	} {
		s = strings.ReplaceAll(s, r[0], r[1])
	}
	s = tagPattern.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, ln := range lines {
		ln = strings.TrimRight(ln, " \t")
		if strings.TrimSpace(ln) == "" {
			blank++
			if blank > 1 {
				continue
			}
			out = append(out, "")
			continue
		}
		blank = 0
		out = append(out, ln)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// tagPattern 匹配残留的标准 HTML 标签（<p>、<span class="x">、</b> 等）。
// 只认「标签名 + 可选属性」的形式，不会吞掉 <Request: ...> 这类带冒号的普通文本。
var tagPattern = regexp.MustCompile(`</?[a-zA-Z][a-zA-Z0-9]*(?:\s[^<>]{0,300})?/?>`)

// openingExcerpt 取开场的第一行做简介，供选择开场时辨认。
func openingExcerpt(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(strings.Trim(strings.TrimSpace(ln), "*_#> "))
		if ln != "" {
			return firstRunes(ln, 60)
		}
	}
	return ""
}

func firstRunes(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func yamlList(items []string) string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it = strings.TrimSpace(it); it != "" {
			out = append(out, yamlString(it))
		}
	}
	return strings.Join(out, ", ")
}

// yamlString 输出可安全放进 YAML 的字符串。
func yamlString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", " ")
	return "\"" + s + "\""
}

// PreviewCard 是角色卡导入的第一步：解析卡、合成作品目录，之后与 zip 导入走同一条校验与暂存流程。
func (im *Importer) PreviewCard(ctx context.Context, name string, r io.Reader) (*ImportPreview, error) {
	// 角色卡是单文件，上限只看 maxCardBytes；多读 1 字节用来判断是否超限，避免把超大文件读进内存。
	limit := int64(maxCardBytes)
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidArgument, "接收上传的角色卡失败")
	}
	if int64(len(data)) > limit {
		return nil, rejectf("角色卡文件过大（上限 %d MiB）", maxCardBytes>>20)
	}
	card, err := ParseSTCard(data)
	if err != nil {
		return nil, err
	}
	set, err := card.FileSet()
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = card.Name + ".png"
	}
	return im.preview(ctx, set, Source{Type: "card", Name: sourceName(name)})
}
