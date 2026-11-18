package runtime_test

import (
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/runtime"
	"github.com/wahhheee/aint/internal/save"
)

func TestRefusalAutoRescueWithoutMarker(t *testing.T) {
	const enabled = "features:\n  force_choices: true\n"
	first := fake.Response{
		Text:       "门后传来了脚步声。",
		Stop:       "refusal",
		ChunkRunes: 1000,
		DelayMS:    1,
	}
	second := choiceResponse()
	second.Text = "脚步声停在了门口。"
	second.ChunkRunes, second.DelayMS = 1000, 1

	var reqs []*llm.Request
	provider := recordingProvider{Provider: fake.New(&fake.Script{Responses: []fake.Response{first, second}}), reqs: &reqs}
	e, err := openEnvWith(t.Context(), t.TempDir(), fakeModels{provider}, func(d *runtime.Deps) {
		d.Budget = runtime.DefaultBudget
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.close() })
	sv := choicesSave(t, e, enabled)
	sub, _, _ := e.bus.Subscribe(event.SaveTopic(sv.ID), 0)
	defer sub.Close()
	if _, err := e.mgr.Submit(t.Context(), sv.ID, save.TurnInput{Kind: save.InputOpening, Text: "推开门"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, sub, event.TurnCommitted)

	if len(reqs) != 2 {
		t.Fatalf("调用 %d 次，应为 2 次", len(reqs))
	}
	last := reqs[1].Messages[len(reqs[1].Messages)-1]
	if last.Role != llm.RoleUser || last.TextOf() != prompts.RefusalRescue {
		t.Fatalf("第二次调用前应追加收束提示：%+v", last)
	}
	tl, err := e.saves.Timeline(t.Context(), sv.ID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	node := tl.Nodes[len(tl.Nodes)-1]
	if node.Stop != "choices" || node.Status == save.StatusInterrupted {
		t.Fatalf("回合结果不符：stop=%s status=%s", node.Stop, node.Status)
	}
	var shown strings.Builder
	for _, it := range node.Display {
		if it.T == "text" {
			shown.WriteString(it.Markup)
		}
	}
	for _, want := range []string{"门后传来了脚步声。", "脚步声停在了门口。"} {
		if !strings.Contains(shown.String(), want) {
			t.Fatalf("正文缺少 %q：%s", want, shown.String())
		}
	}
	if strings.Contains(shown.String(), "抱歉") || strings.Contains(shown.String(), "我不能") {
		t.Fatalf("正文不应包含拒绝语：%s", shown.String())
	}
}
