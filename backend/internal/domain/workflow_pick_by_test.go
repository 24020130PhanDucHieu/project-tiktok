package domain

import (
	"strings"
	"testing"
)

func pickGraph(mut func(n *WorkflowNode)) []WorkflowNode {
	n := WorkflowNode{ID: "p", Type: NodePick}
	mut(&n)
	return []WorkflowNode{n}
}

func TestAPickBlockMayChooseKocsByNameOrFilterBySourceAlone(t *testing.T) {
	id1, id2 := NewID().Hex(), NewID().Hex()
	ok := []func(n *WorkflowNode){
		func(n *WorkflowNode) { n.KocIDs = []string{id1, id2} },
		func(n *WorkflowNode) { n.Source, n.Limit = "TikTok", 5 },
	}
	for i, m := range ok {
		if err := ValidateWorkflowGraph(pickGraph(m), nil); err != nil {
			t.Fatalf("case %d should be valid: %v", i, err)
		}
	}
	bad := map[string]func(n *WorkflowNode){
		"không hợp lệ": func(n *WorkflowNode) { n.KocIDs = []string{"abc"} },
		"hai lần":      func(n *WorkflowNode) { n.KocIDs = []string{id1, id1} },
		"tối đa 200":   func(n *WorkflowNode) { n.KocIDs = manyIDs(201) },
	}
	for want, m := range bad {
		err := ValidateWorkflowGraph(pickGraph(m), nil)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%q expected, got %v", want, err)
		}
	}
}

func manyIDs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = NewID().Hex()
	}
	return out
}

func TestSanitizeKeepsTheSourceAndTheChosenKocsOfAPickBlockOnly(t *testing.T) {
	id := NewID().Hex()
	pick := SanitizeNode(WorkflowNode{ID: "p", Type: NodePick, Source: "TikTok", KocIDs: []string{id}})
	if pick.Source != "TikTok" || len(pick.KocIDs) != 1 {
		t.Fatalf("a pick block keeps them: %+v", pick)
	}
	wait := SanitizeNode(WorkflowNode{ID: "w", Type: NodeWait, WaitMinutes: 5, Source: "TikTok", KocIDs: []string{id}})
	if wait.Source != "" || len(wait.KocIDs) != 0 {
		t.Fatalf("any other block drops them: %+v", wait)
	}
}

func TestAStageBlockMayMoveKocsBackToSourced(t *testing.T) {
	g := []WorkflowNode{{ID: "p", Type: NodePick, Stages: []string{StageDormant}, Limit: 1}, {ID: "s", Type: NodeStage, Stage: StageSourced}}
	e := []WorkflowEdge{{ID: "e", From: "p", Port: PortOut, To: "s"}}
	if err := ValidateWorkflowGraph(g, e); err != nil {
		t.Fatalf("Mới tìm is a stage a block may set: %v", err)
	}
}
