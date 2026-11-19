package service

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

type fakeNotifier struct {
	mu    sync.Mutex
	calls []domain.Workflow
}

func (f *fakeNotifier) Notify(_ context.Context, wf *domain.Workflow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, *wf)
}

type fakeSubjectMailer struct {
	to, subject, text string
	calls             int
}

func (f *fakeSubjectMailer) Send(to, subject, text string) error {
	f.calls++
	f.to, f.subject, f.text = to, subject, text
	return nil
}

func doneWorkflow() *domain.Workflow {
	return &domain.Workflow{
		Name: "Test_01", By: "viet@x.vn", Status: domain.WorkflowDone,
		Nodes:  []domain.WorkflowNode{{ID: "p", Type: domain.NodePick}, {ID: "s", Type: domain.NodeStage, Stage: domain.StageDormant}},
		States: []domain.NodeState{{NodeID: "p", Status: domain.NodeDone, Count: 3}, {NodeID: "s", Status: domain.NodeDone, Count: 3, Reason: "2 đã đổi, 1 bỏ qua"}},
	}
}

func TestTheMailSaysWhatAFinishedWorkflowDidBlockByBlockAndWhereToLook(t *testing.T) {
	m := &fakeSubjectMailer{}
	NewWorkflowMailNotifier(m, "https://app.example").Notify(context.Background(), doneWorkflow())
	if m.calls != 1 || m.to != "viet@x.vn" {
		t.Fatalf("one mail to the person who started it: %+v", m)
	}
	if !strings.Contains(m.subject, "Test_01") || !strings.Contains(m.subject, "đã xong") {
		t.Fatalf("the subject names the workflow and says it is done: %q", m.subject)
	}
	for _, want := range []string{"Lấy KOC có sẵn: 3 KOC", "Đổi giai đoạn: 3 KOC (2 đã đổi, 1 bỏ qua)", "https://app.example"} {
		if !strings.Contains(m.text, want) {
			t.Fatalf("the mail says %q:\n%s", want, m.text)
		}
	}
}

func TestTheMailSaysWhyAStoppedWorkflowStoppedAndSendsNothingToAnAddressThatIsNotOne(t *testing.T) {
	m := &fakeSubjectMailer{}
	wf := doneWorkflow()
	wf.Status, wf.StopReason = domain.WorkflowStopped, "send_stopped:zalo_errors"
	NewWorkflowMailNotifier(m, "").Notify(context.Background(), wf)
	if !strings.Contains(m.subject, "đã dừng") || !strings.Contains(m.text, "Zalo lỗi nhiều lần") {
		t.Fatalf("stopped, and why: %q\n%s", m.subject, m.text)
	}
	m2 := &fakeSubjectMailer{}
	wf.By = "admin"
	NewWorkflowMailNotifier(m2, "").Notify(context.Background(), wf)
	if m2.calls != 0 {
		t.Fatal("a name that is not an email address gets no mail")
	}
}

func TestTheRunnerTellsTheNotifierOnceWhenTheWorkflowEnds(t *testing.T) {
	e := newWfEnv()
	n := &fakeNotifier{}
	e.r.WithNotifier(n)
	nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), wwait("w", 5)}
	e.begin(nodes, []domain.WorkflowEdge{wedge("i", domain.PortOut, "w")}, domain.WorkflowToken{NodeID: "w", Kocs: tokenKocs("k0"), ReadyAt: e.clock})
	if len(n.calls) != 1 || n.calls[0].Status != domain.WorkflowDone {
		t.Fatalf("one call, with the workflow as it ended: %+v", n.calls)
	}
}
