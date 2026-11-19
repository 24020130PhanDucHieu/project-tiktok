package service

import (
	"testing"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// The switch of "Gửi hàng loạt" is the stop button of the run that is running. A branch that has to wait for that run must
// not turn the switch on again, or it would undo a stop.
func TestABusyBranchLeavesTheSwitchAloneWhileAnotherRunRuns(t *testing.T) {
	e := newWfEnv()
	nodes := []domain.WorkflowNode{wnode("i", domain.NodeImport), wnode("q", domain.NodeIfReplied), wsend("sA"), wsend("sB")}
	edges := []domain.WorkflowEdge{wedge("i", domain.PortOut, "q"), wedge("q", domain.PortReplied, "sA"), wedge("q", domain.PortUnanswered, "sB")}
	var switchAfterStop []bool
	e.onNap = func(n int) { // the run of branch A never ends here
		if n == 4 {
			e.sw.on = false // the person turns the switch off to stop the run
		}
		if n > 4 && n < 12 {
			switchAfterStop = append(switchAfterStop, e.sw.on)
		}
		if n == 12 {
			e.store.askToStop()
		}
	}
	e.begin(nodes, edges,
		domain.WorkflowToken{NodeID: "sA", Kocs: kocTokens(3), ReadyAt: e.clock},
		domain.WorkflowToken{NodeID: "sB", Kocs: []domain.TokenKoc{{KocID: "b0"}}, ReadyAt: e.clock})
	if len(switchAfterStop) == 0 {
		t.Fatal("the test did not reach the point where the person turns the switch off")
	}
	for i, on := range switchAfterStop {
		if on {
			t.Fatalf("tick %d: the waiting branch turned the switch back on, undoing the stop", i)
		}
	}
	if len(e.runs.starts) != 1 {
		t.Fatalf("only the first branch ever started a run: %d", len(e.runs.starts))
	}
}
