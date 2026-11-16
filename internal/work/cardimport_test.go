package work

import (
	"strings"
	"testing"
)

func TestConvertCardTags(t *testing.T) {
	in := "<N>阿琳</N>" + "\n" + "<A>36岁</A>" + "\n" + "<O>村民 / 妻子</O>" + "\n" +
		"<C>穿着一件红色毛衣，袖口卷到手肘。</C>" + "\n" +
		"<H>100/100</H><S>65/100</S><P>5/100</P><D>95/100</D>" + "\n" +
		"<T>08:15 AM</T>" + "\n" + "<DT>冬天/过年当天</DT>" + "\n" + "<L>你的家/卧室</L>" + "\n"
	got := cleanText(convertCardTags(in))
	t.Logf("\n%s", got)
	for _, want := range []string{
		"**阿琳（36岁）**",
		"*村民 / 妻子*",
		"> 穿着一件红色毛衣，袖口卷到手肘。",
		"H 100/100　S 65/100　P 5/100　D 95/100",
		"08:15 AM｜冬天/过年当天｜你的家/卧室",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("缺少 %q\n实际输出：\n%s", want, got)
		}
	}
	if strings.Contains(got, "<") {
		t.Errorf("仍有残留标签：%s", got)
	}
}
