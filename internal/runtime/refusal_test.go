package runtime

import "testing"

func TestRefusalNarrative(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "保留拒答前的完整叙事",
			text: "门后传来了脚步声。\n\n抱歉，我不能继续写这个内容。",
			want: "门后传来了脚步声。",
		},
		{
			name: "纯拒答不保留",
			text: "抱歉，我不能继续写这个内容。",
			want: "",
		},
		{
			name: "叙事中的不能不是拒答",
			text: "她不能相信自己的眼睛。\n\n很抱歉，我不能继续。",
			want: "她不能相信自己的眼睛。",
		},
		{
			name: "英文叙事前缀",
			text: "The door creaked open. I'm sorry, but I can't continue.",
			want: "The door creaked open.",
		},
		{
			name: "未知拒答不猜正文",
			text: "The model stopped here.",
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := refusalNarrative(tc.text); got != tc.want {
				t.Fatalf("refusalNarrative(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}
