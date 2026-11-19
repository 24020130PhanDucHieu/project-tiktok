package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// withBlockStores gives the flow environment the stores of the new blocks, built for the three KOCs it knows.
func (f *flowEnv) withBlockStores() {
	f.picker = &fixedPicker{kocs: tokenKocs(f.kocs...)}
	f.labels = newFakeLabels()
	f.stages = newFakeStages()
	f.emailer = newFakeEmailer(func() time.Time { return f.we.clock })
	for i, id := range f.kocs {
		f.labels.koc(id, id, true)
		_ = i
		f.stages.add(id)
	}
	f.r = f.runner(nil)
}

func TestSilentKocsAreRemindedAndTheOneWhoAnsweredIsMoved(t *testing.T) {
	f := newFlowEnv()
	f.withBlockStores()
	f.we.replies.answer(f.kocs[1], true, true) // KOC 1 answers after the first text

	first, reminder := sendTextNode("t1"), sendTextNode("t2")
	first.Text, reminder.Text = "Chào {ten}, anh chị xem giúp em nhé", "Nhắc nhẹ: anh chị xem tin hôm trước giúp em nhé"
	nodes := []domain.WorkflowNode{
		{ID: "p", Type: domain.NodePick, Stages: []string{"CONTACTED"}, Limit: 10},
		first, wwait("w", 2*24*60), wnode("q", domain.NodeIfReplied), reminder,
		{ID: "l", Type: domain.NodeLabel, LabelID: testLabel},
		{ID: "g", Type: domain.NodeStage, Stage: domain.StageReplied},
	}
	edges := []domain.WorkflowEdge{
		wedge("p", domain.PortOut, "t1"), wedge("t1", domain.PortOut, "w"), wedge("w", domain.PortOut, "q"),
		wedge("q", domain.PortUnanswered, "t2"), wedge("t2", domain.PortOut, "l"), wedge("q", domain.PortReplied, "g"),
	}
	if _, err := f.r.Start(context.Background(), "admin@t.t", &domain.Workflow{Name: "Nhắc KOC im lặng", Nodes: nodes, Edges: edges}); err != nil {
		t.Fatal(err)
	}
	f.waitAll()

	got := f.we.store.only()
	if got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
	if len(f.re.runs.runs) != 2 {
		t.Fatalf("the text and the reminder: %d runs", len(f.re.runs.runs))
	}
	r1, r2 := f.re.runs.runs[0], f.re.runs.runs[1]
	if len(r1.Targets) != 3 || len(r2.Targets) != 2 || r2.Targets[0].KocID != f.kocs[0] || r2.Targets[1].KocID != f.kocs[2] {
		t.Fatalf("everybody gets the text, only the two who were silent get the reminder: %d then %+v", len(r1.Targets), r2.Targets)
	}
	if r1.Steps[0].Text != first.Text || r2.Steps[0].Text != reminder.Text {
		t.Fatalf("each run sends its own text: %q and %q", r1.Steps[0].Text, r2.Steps[0].Text)
	}
	if len(f.re.line.calls) != 5 {
		t.Fatalf("5 messages in all: %d", len(f.re.line.calls))
	}
	// the label goes to the two who were reminded, the stage change to the one who answered
	labeled := 0
	for _, id := range f.kocs {
		conv := f.labels.convs["@"+id]
		if len(f.labels.added[conv.ID]) == 1 {
			labeled++
		}
	}
	if labeled != 2 || len(f.labels.added[f.labels.convs["@"+f.kocs[1]].ID]) != 0 {
		t.Fatalf("the two reminded KOCs are labelled, the one who answered is not: %+v", f.labels.added)
	}
	if len(f.stages.applied) != 1 || f.stages.applied[f.kocs[1]] != domain.StageReplied {
		t.Fatalf("only the KOC who answered is moved: %+v", f.stages.applied)
	}
	if f.elapsed() < 2*24*time.Hour {
		t.Fatalf("the two days were waited: %v", f.elapsed())
	}
}

func TestCareAfterASampleWaitsForNineThenTextsThenEmails(t *testing.T) {
	f := newFlowEnv()
	f.withBlockStores()

	ask := sendTextNode("t")
	ask.Text = "Anh chị nhận được hàng chưa ạ?"
	nodes := []domain.WorkflowNode{
		{ID: "p", Type: domain.NodePick, Stages: []string{"SHIPPING"}, Limit: 10},
		{ID: "u", Type: domain.NodeWaitUntil, AtMinute: 9 * 60},
		ask, wwait("w", 3*24*60),
		{ID: "m", Type: domain.NodeEmail, Subject: "Nhắc đăng video", Text: "Chào {ten}, bao giờ mình đăng video giúp em nhé"},
	}
	edges := []domain.WorkflowEdge{wedge("p", domain.PortOut, "u"), wedge("u", domain.PortOut, "t"), wedge("t", domain.PortOut, "w"), wedge("w", domain.PortOut, "m")}
	var nineWait *time.Time
	f.we.onNap = nil
	f.r = f.runner(func(int) {
		if s, ok := stateOfWorkflow(f, "u"); ok && s.Status == domain.NodeWaiting && nineWait == nil {
			nineWait = s.WaitUntil
		}
	})
	if _, err := f.r.Start(context.Background(), "admin@t.t", &domain.Workflow{Name: "Chăm sóc sau gửi mẫu", Nodes: nodes, Edges: edges}); err != nil {
		t.Fatal(err)
	}
	f.waitAll()

	got := f.we.store.only()
	if got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
	nine := time.Date(2026, 11, 21, 9, 0, 0, 0, vnZone)
	if nineWait == nil || !nineWait.Equal(nine) {
		t.Fatalf("the workflow waited for 09:00 tomorrow: %v", nineWait)
	}
	if len(f.re.runs.runs) != 1 || len(f.re.runs.runs[0].Targets) != 3 || len(f.re.line.calls) != 3 {
		t.Fatalf("one text to each of the three KOCs: %d runs, %d messages", len(f.re.runs.runs), len(f.re.line.calls))
	}
	if strings.Join(f.emailer.sent, ",") != strings.Join(f.kocs, ",") {
		t.Fatalf("then an email to each of them, in order: %v", f.emailer.sent)
	}
	if first := f.emailer.times[0]; first.Before(nine.Add(3 * 24 * time.Hour)) {
		t.Fatalf("the emails go out three days after the text, not at %v", first)
	}
	for i := 1; i < len(f.emailer.times); i++ {
		if f.emailer.times[i].Sub(f.emailer.times[i-1]) < 5*time.Second {
			t.Fatal("two emails are never closer than the pause")
		}
	}
	if f.emailer.subjects[0] != "Nhắc đăng video" {
		t.Fatalf("the subject: %q", f.emailer.subjects[0])
	}
}

func stateOfWorkflow(f *flowEnv, nodeID string) (domain.NodeState, bool) {
	for _, s := range f.we.store.only().States {
		if s.NodeID == nodeID {
			return s, true
		}
	}
	return domain.NodeState{}, false
}

func TestARestartWhileTheEmailsGoOutSendsEachKocOneEmailInAll(t *testing.T) {
	f := newFlowEnv()
	f.withBlockStores()
	nodes := []domain.WorkflowNode{
		{ID: "p", Type: domain.NodePick, Stages: []string{"SHIPPING"}, Limit: 10},
		{ID: "m", Type: domain.NodeEmail, Subject: "S", Text: "Chào {ten}"},
	}
	edges := []domain.WorkflowEdge{wedge("p", domain.PortOut, "m")}
	f.r = f.runner(func(int) {
		if len(f.emailer.sent) == 1 { // the server dies after the first email
			die()
		}
	})
	if _, err := f.r.Start(context.Background(), "admin@t.t", &domain.Workflow{Nodes: nodes, Edges: edges}); err != nil {
		t.Fatal(err)
	}
	f.restart()
	if strings.Join(f.emailer.sent, ",") != strings.Join(f.kocs, ",") {
		t.Fatalf("every KOC got exactly one email, none twice and none missed: %v", f.emailer.sent)
	}
	if got := f.we.store.only(); got.Status != domain.WorkflowDone {
		t.Fatalf("%s %q", got.Status, got.StopReason)
	}
}
