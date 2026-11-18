package runtime_test

import (
	"strings"
	"testing"
	"time"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/llm/fake"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/runtime"
	"github.com/wahhheee/aint/internal/save"
)

func TestRefusalAutoRescue(t *testing.T) {
	const enabled = "features:\n  force_choices: true\n"
	first := fake.Response{
		Text:       "门后传来了脚步声。\n\n抱歉，我不能继续写这个内容。",
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
	var reset *event.TurnResetData
	committed := false
	deadline := time.After(5 * time.Second)
	for !committed {
		select {
		case e := <-sub.C:
			switch e.Type {
			case event.TurnReset:
				v := e.Data.(event.TurnResetData)
				reset = &v
			case event.TurnCommitted:
				committed = true
			case event.TurnFailed:
				t.Fatalf("回合失败：%+v", e.Data)
			}
		case <-deadline:
			t.Fatal("等待回合提交超时")
		}
	}
	if reset == nil {
		t.Fatal("未收到 turn.reset")
	}
	items, ok := reset.Items.([]save.DisplayItem)
	if !ok {
		t.Fatalf("turn.reset 载荷类型不符：%T", reset.Items)
	}
	var resetShown strings.Builder
	for _, it := range items {
		if it.T == "text" {
			resetShown.WriteString(it.Markup)
		}
	}
	for _, bad := range []string{"抱歉", "我不能"} {
		if strings.Contains(resetShown.String(), bad) {
			t.Fatalf("重置后的正文不应包含拒绝语 %q：%s", bad, resetShown.String())
		}
	}
	if !strings.Contains(resetShown.String(), "门后传来了脚步声。") {
		t.Fatalf("重置后的正文缺少已写内容：%s", resetShown.String())
	}

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
	hasChoices := false
	for _, it := range node.Display {
		switch it.T {
		case "text":
			shown.WriteString(it.Markup)
		case "choices":
			hasChoices = true
		}
	}
	if !hasChoices {
		t.Fatalf("缺少选项：%+v", node.Display)
	}
	for _, bad := range []string{"抱歉", "我不能"} {
		if strings.Contains(shown.String(), bad) {
			t.Fatalf("正文不应包含拒绝语 %q：%s", bad, shown.String())
		}
	}
	for _, want := range []string{"门后传来了脚步声。", "脚步声停在了门口。"} {
		if !strings.Contains(shown.String(), want) {
			t.Fatalf("正文缺少 %q：%s", want, shown.String())
		}
	}

	_, line, err := e.saves.ProgressLine(t.Context(), sv.ID)
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := e.saves.LoadMessages(t.Context(), line[len(line)-1].MessagesHash)
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range msgs {
		if strings.Contains(msg.TextOf(), "抱歉") || strings.Contains(msg.TextOf(), "我不能") {
			t.Fatalf("历史中不应保留拒绝语：%+v", msg)
		}
	}
}

func TestRefusalAutoRescueWithToolCall(t *testing.T) {
	const enabled = "features:\n  force_choices: true\n"
	first := choiceResponse()
	first.Text = "门后传来了脚步声。\n\n抱歉，我不能继续写这个内容。"
	first.Stop = "refusal"
	first.ChunkRunes, first.DelayMS = 1000, 1
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
}
