package domain

import (
	"fmt"
	"strings"
	"testing"
)

func node(id, typ string) WorkflowNode {
	n := WorkflowNode{ID: id, Type: typ}
	switch typ {
	case NodeWait:
		n.WaitMinutes = 60
	case NodeSend:
		n.CampaignID = "c1"
	}
	return n
}

func edge(from, port, to string) WorkflowEdge {
	return WorkflowEdge{ID: from + ">" + to, From: from, Port: port, To: to}
}

func TestValidateWorkflowGraph(t *testing.T) {
	longChain := []WorkflowNode{node("n0", NodeImport)}
	var longEdges []WorkflowEdge
	for i := 1; i <= 15; i++ { // 16 blocks in all
		longChain = append(longChain, node(fmt.Sprintf("n%d", i), NodeWait))
		longEdges = append(longEdges, edge(fmt.Sprintf("n%d", i-1), PortOut, fmt.Sprintf("n%d", i)))
	}
	wait := func(id string, minutes int) WorkflowNode { n := node(id, NodeWait); n.WaitMinutes = minutes; return n }
	send := func(id, campaign string) WorkflowNode { n := node(id, NodeSend); n.CampaignID = campaign; return n }

	cases := []struct {
		name  string
		nodes []WorkflowNode
		edges []WorkflowEdge
		want  string // "" = valid, otherwise a word the message must contain
	}{
		{"a chain", []WorkflowNode{node("i", NodeImport), node("w", NodeWait), node("s", NodeSend)},
			[]WorkflowEdge{edge("i", PortOut, "w"), edge("w", PortOut, "s")}, ""},
		{"a split with both ports connected",
			[]WorkflowNode{node("i", NodeImport), node("s", NodeSend), node("q", NodeIfReplied), node("s2", NodeSend), node("w", NodeWait)},
			[]WorkflowEdge{edge("i", PortOut, "s"), edge("s", PortOut, "q"), edge("q", PortUnanswered, "s2"), edge("q", PortReplied, "w")}, ""},
		{"a split with one port connected",
			[]WorkflowNode{node("i", NodeImport), node("s", NodeSend), node("q", NodeIfReplied), node("s2", NodeSend)},
			[]WorkflowEdge{edge("i", PortOut, "s"), edge("s", PortOut, "q"), edge("q", PortUnanswered, "s2")}, ""},
		{"only the import block", []WorkflowNode{node("i", NodeImport)}, nil, ""},

		{"no import block", []WorkflowNode{node("w", NodeWait)}, nil, "chưa có khối Nhập"},
		{"two import blocks", []WorkflowNode{node("a", NodeImport), node("b", NodeImport)}, nil, "chỉ được có một khối Nhập"},
		{"a block that is not reached", []WorkflowNode{node("i", NodeImport), node("w", NodeWait)}, nil, "chưa được nối"},
		{"two wires into one block",
			[]WorkflowNode{node("i", NodeImport), node("q", NodeIfReplied), node("w", NodeWait)},
			[]WorkflowEdge{edge("i", PortOut, "q"), edge("q", PortReplied, "w"), edge("q", PortUnanswered, "w")}, "nhiều hơn một dây vào"},
		{"two wires out of one port",
			[]WorkflowNode{node("i", NodeImport), node("a", NodeWait), node("b", NodeWait)},
			[]WorkflowEdge{edge("i", PortOut, "a"), edge("i", PortOut, "b")}, "nhiều hơn một dây ra"},
		{"a cycle", []WorkflowNode{node("i", NodeImport), node("a", NodeWait), node("b", NodeWait)},
			[]WorkflowEdge{edge("a", PortOut, "b"), edge("b", PortOut, "a")}, "vòng"},
		{"a wire into the import block", []WorkflowNode{node("i", NodeImport), node("w", NodeWait)},
			[]WorkflowEdge{edge("i", PortOut, "w"), edge("w", PortOut, "i")}, "không nhận dây vào"},
		{"a wire to a block that does not exist", []WorkflowNode{node("i", NodeImport)},
			[]WorkflowEdge{edge("i", PortOut, "ghost")}, "không tồn tại"},
		{"a port the block does not have", []WorkflowNode{node("i", NodeImport), node("w", NodeWait), node("s", NodeSend)},
			[]WorkflowEdge{edge("i", PortOut, "w"), edge("w", PortReplied, "s")}, "cổng"},
		{"two blocks with the same id", []WorkflowNode{node("i", NodeImport), node("i", NodeWait)}, nil, "trùng"},
		{"a block of an unknown type", []WorkflowNode{node("i", NodeImport), node("x", "email")},
			[]WorkflowEdge{edge("i", PortOut, "x")}, "loại khối"},
		{"a wait of zero minutes", []WorkflowNode{node("i", NodeImport), wait("w", 0)},
			[]WorkflowEdge{edge("i", PortOut, "w")}, "thời gian chờ"},
		{"a wait of fourteen days and a minute", []WorkflowNode{node("i", NodeImport), wait("w", MaxWaitMinutes+1)},
			[]WorkflowEdge{edge("i", PortOut, "w")}, "thời gian chờ"},
		{"a wait of exactly fourteen days", []WorkflowNode{node("i", NodeImport), wait("w", MaxWaitMinutes)},
			[]WorkflowEdge{edge("i", PortOut, "w")}, ""},
		{"a send block with no campaign", []WorkflowNode{node("i", NodeImport), send("s", "")},
			[]WorkflowEdge{edge("i", PortOut, "s")}, "chiến dịch"},
		{"sixteen blocks", longChain, longEdges, "tối đa 15"},
		{"the waits of one branch add up past fourteen days",
			[]WorkflowNode{node("i", NodeImport), wait("a", 10*24*60), wait("b", 5*24*60)},
			[]WorkflowEdge{edge("i", PortOut, "a"), edge("a", PortOut, "b")}, "tổng thời gian chờ"},
		{"two branches that each wait a long time are fine",
			[]WorkflowNode{node("i", NodeImport), node("q", NodeIfReplied), wait("a", 10*24*60), wait("b", 10*24*60)},
			[]WorkflowEdge{edge("i", PortOut, "q"), edge("q", PortReplied, "a"), edge("q", PortUnanswered, "b")}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateWorkflowGraph(tc.nodes, tc.edges)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("a valid graph was refused: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("an invalid graph was accepted (want a message with %q)", tc.want)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
				t.Fatalf("message %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestNextNodeFollowsTheWireOfAPort(t *testing.T) {
	edges := []WorkflowEdge{edge("q", PortReplied, "a"), edge("q", PortUnanswered, "b")}
	if to, ok := NextNode(edges, "q", PortUnanswered); !ok || to != "b" {
		t.Fatalf("unanswered leads to %q (%v), want b", to, ok)
	}
	if _, ok := NextNode(edges, "q", PortOut); ok {
		t.Fatal("a port with no wire must lead nowhere")
	}
}

func TestMaxPathWaitMinutesTakesTheLongestBranch(t *testing.T) {
	nodes := []WorkflowNode{node("i", NodeImport), node("q", NodeIfReplied), {ID: "a", Type: NodeWait, WaitMinutes: 100}, {ID: "b", Type: NodeWait, WaitMinutes: 30}, {ID: "c", Type: NodeWait, WaitMinutes: 50}}
	edges := []WorkflowEdge{edge("i", PortOut, "q"), edge("q", PortReplied, "a"), edge("q", PortUnanswered, "b"), edge("b", PortOut, "c")}
	if got := MaxPathWaitMinutes(nodes, edges); got != 100 {
		t.Fatalf("longest branch waits %d minutes, want 100 (the other branch waits 30+50=80)", got)
	}
}
