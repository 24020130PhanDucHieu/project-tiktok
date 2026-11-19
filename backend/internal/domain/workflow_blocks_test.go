package domain

import (
	"strings"
	"testing"
)

func cfg(id, typ string, set func(*WorkflowNode)) WorkflowNode {
	n := WorkflowNode{ID: id, Type: typ}
	set(&n)
	return n
}

func chain(nodes ...WorkflowNode) ([]WorkflowNode, []WorkflowEdge) {
	var edges []WorkflowEdge
	for i := 1; i < len(nodes); i++ {
		edges = append(edges, edge(nodes[i-1].ID, PortOut, nodes[i].ID))
	}
	return nodes, edges
}

func TestValidateTheSixNewBlocks(t *testing.T) {
	pick := cfg("p", NodePick, func(n *WorkflowNode) { n.Stages = []string{"SHIPPING"}; n.Limit = 50 })
	start := node("i", NodeImport)
	long := strings.Repeat("a", 2001)

	valid := map[string][]WorkflowNode{
		"pick then wait until then a text":  {pick, cfg("u", NodeWaitUntil, func(n *WorkflowNode) { n.AtMinute = 9 * 60 }), cfg("t", NodeSendText, func(n *WorkflowNode) { n.Text = "Chào {ten}" })},
		"a pick by folder only":             {cfg("p", NodePick, func(n *WorkflowNode) { n.Folder = "Làm đẹp"; n.Limit = 10 })},
		"label and stage after an import":   {start, cfg("l", NodeLabel, func(n *WorkflowNode) { n.LabelID = "65a1b2c3d4e5f60718293a4b" }), cfg("s", NodeStage, func(n *WorkflowNode) { n.Stage = "NO_REPLY" })},
		"an email with the default subject": {start, cfg("e", NodeEmail, func(n *WorkflowNode) { n.Text = "Xin chào" })},
		"an email with a subject":           {start, cfg("e", NodeEmail, func(n *WorkflowNode) { n.Text = "Xin chào"; n.Subject = "Hợp tác" })},
		"midnight is a valid time to wait":  {start, cfg("u", NodeWaitUntil, func(n *WorkflowNode) { n.AtMinute = 0 })},
		"the last minute of the day":        {start, cfg("u", NodeWaitUntil, func(n *WorkflowNode) { n.AtMinute = 1439 })},
		"text of exactly 2000 characters":   {start, cfg("t", NodeSendText, func(n *WorkflowNode) { n.Text = strings.Repeat("ạ", 2000) })},
	}
	for name, nodes := range valid {
		t.Run("valid: "+name, func(t *testing.T) {
			n, e := chain(nodes...)
			if err := ValidateWorkflowGraph(n, e); err != nil {
				t.Fatalf("a valid graph was refused: %v", err)
			}
		})
	}

	invalid := []struct {
		name  string
		nodes []WorkflowNode
		want  string
	}{
		{"import and pick together", []WorkflowNode{start, pick}, "chỉ được có một khối bắt đầu"},
		{"no start block", []WorkflowNode{cfg("w", NodeWait, func(n *WorkflowNode) { n.WaitMinutes = 5 })}, "chưa có khối bắt đầu"},
		{"wait until after the end of the day", []WorkflowNode{start, cfg("u", NodeWaitUntil, func(n *WorkflowNode) { n.AtMinute = 1440 })}, "giờ chờ"},
		{"wait until before midnight", []WorkflowNode{start, cfg("u", NodeWaitUntil, func(n *WorkflowNode) { n.AtMinute = -1 })}, "giờ chờ"},
		{"a text with no content", []WorkflowNode{start, cfg("t", NodeSendText, func(n *WorkflowNode) {})}, "nội dung"},
		{"a text that is too long", []WorkflowNode{start, cfg("t", NodeSendText, func(n *WorkflowNode) { n.Text = long })}, "nội dung"},
		{"an email with no content", []WorkflowNode{start, cfg("e", NodeEmail, func(n *WorkflowNode) {})}, "nội dung"},
		{"an email with a subject that is too long", []WorkflowNode{start, cfg("e", NodeEmail, func(n *WorkflowNode) { n.Text = "x"; n.Subject = strings.Repeat("a", 151) })}, "chủ đề"},
		{"a label that is not an id", []WorkflowNode{start, cfg("l", NodeLabel, func(n *WorkflowNode) { n.LabelID = "nhan" })}, "nhãn"},
		{"a label with no id", []WorkflowNode{start, cfg("l", NodeLabel, func(n *WorkflowNode) {})}, "nhãn"},
		{"a stage that is not offered", []WorkflowNode{start, cfg("s", NodeStage, func(n *WorkflowNode) { n.Stage = "COMPLETED" })}, "giai đoạn"},
		{"no stage at all", []WorkflowNode{start, cfg("s", NodeStage, func(n *WorkflowNode) {})}, "giai đoạn"},
		{"a pick with neither a stage nor a folder", []WorkflowNode{cfg("p", NodePick, func(n *WorkflowNode) { n.Limit = 10 })}, "giai đoạn hoặc thư mục"},
		{"a pick with a stage that does not exist", []WorkflowNode{cfg("p", NodePick, func(n *WorkflowNode) { n.Stages = []string{"NOPE"}; n.Limit = 10 })}, "giai đoạn"},
		{"a pick with six stages", []WorkflowNode{cfg("p", NodePick, func(n *WorkflowNode) {
			n.Stages = []string{"SOURCED", "QUALIFIED", "CONTACTED", "REPLIED", "AGREED", "SHIPPING"}
			n.Limit = 10
		})}, "giai đoạn"},
		{"a pick with a limit of zero", []WorkflowNode{cfg("p", NodePick, func(n *WorkflowNode) { n.Stages = []string{"SHIPPING"} })}, "giới hạn"},
		{"a pick with a limit over 200", []WorkflowNode{cfg("p", NodePick, func(n *WorkflowNode) { n.Stages = []string{"SHIPPING"}; n.Limit = 201 })}, "giới hạn"},
	}
	for _, tc := range invalid {
		t.Run("invalid: "+tc.name, func(t *testing.T) {
			n, e := chain(tc.nodes...)
			if tc.name == "import and pick together" {
				e = nil // two blocks that both start the workflow, not wired
			}
			err := ValidateWorkflowGraph(n, e)
			if err == nil {
				t.Fatalf("an invalid graph was accepted (want a message with %q)", tc.want)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("message %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestTwentyFiveBlocksAreAllowedAndTwentySixAreNot(t *testing.T) {
	build := func(n int) ([]WorkflowNode, []WorkflowEdge) {
		nodes := []WorkflowNode{node("n0", NodeImport)}
		var edges []WorkflowEdge
		for i := 1; i < n; i++ {
			nodes = append(nodes, WorkflowNode{ID: "n" + string(rune('a'+i)), Type: NodeLabel, LabelID: "65a1b2c3d4e5f60718293a4b"})
			edges = append(edges, edge(nodes[i-1].ID, PortOut, nodes[i].ID))
		}
		return nodes, edges
	}
	if n, e := build(25); ValidateWorkflowGraph(n, e) != nil {
		t.Fatalf("25 blocks must be allowed: %v", ValidateWorkflowGraph(n, e))
	}
	n, e := build(26)
	if err := ValidateWorkflowGraph(n, e); err == nil || !strings.Contains(err.Error(), "tối đa 25") {
		t.Fatalf("26 blocks must be refused: %v", err)
	}
}

func TestAWaitUntilCountsAsADayInTheTotalWait(t *testing.T) {
	nodes := []WorkflowNode{node("i", NodeImport), cfg("u", NodeWaitUntil, func(n *WorkflowNode) { n.AtMinute = 540 }), {ID: "w", Type: NodeWait, WaitMinutes: 3 * 24 * 60}}
	n, e := chain(nodes...)
	if got := MaxPathWaitMinutes(n, e); got != 24*60+3*24*60 {
		t.Fatalf("one day for the clock time plus three days: %d minutes", got)
	}
	nodes = []WorkflowNode{node("i", NodeImport), {ID: "w", Type: NodeWait, WaitMinutes: MaxWaitMinutes}, cfg("u", NodeWaitUntil, func(n *WorkflowNode) { n.AtMinute = 540 })}
	n, e = chain(nodes...)
	if err := ValidateWorkflowGraph(n, e); err == nil || !strings.Contains(strings.ToLower(err.Error()), "tổng thời gian chờ") {
		t.Fatalf("fourteen days of waiting plus a wait until is over the limit: %v", err)
	}
}

func TestSanitizeNodeKeepsOnlyWhatTheTypeUses(t *testing.T) {
	dirty := WorkflowNode{
		ID: "x", Type: NodeWait, X: 3, Y: 4, WaitMinutes: 60, AtMinute: 9, CampaignID: "c", CampaignName: "n",
		Steps: []CampaignStep{{Kind: StepText, Text: "hack"}}, Text: "t", Subject: "s", LabelID: "l", Stage: "NO_REPLY", Stages: []string{"A"}, Folder: "f", Limit: 5,
	}
	got := SanitizeNode(dirty)
	if got.ID != "x" || got.Type != NodeWait || got.X != 3 || got.Y != 4 || got.WaitMinutes != 60 {
		t.Fatalf("the identity, the place and the settings of the type are kept: %+v", got)
	}
	if got.AtMinute != 0 || got.CampaignID != "" || got.CampaignName != "" || got.Steps != nil || got.Text != "" || got.Subject != "" || got.LabelID != "" || got.Stage != "" || got.Stages != nil || got.Folder != "" || got.Limit != 0 {
		t.Fatalf("a wait block keeps nothing that belongs to another type: %+v", got)
	}
	send := SanitizeNode(WorkflowNode{ID: "s", Type: NodeSend, CampaignID: "c1", CampaignName: "browser", Steps: []CampaignStep{{Kind: StepText, Text: "hack"}}})
	if send.CampaignID != "c1" || send.CampaignName != "" || send.Steps != nil {
		t.Fatalf("a send block keeps its campaign id but never a name or steps from the browser: %+v", send)
	}
	pick := SanitizeNode(WorkflowNode{ID: "p", Type: NodePick, Stages: []string{"SHIPPING"}, Folder: "f", Limit: 9, Text: "x"})
	if len(pick.Stages) != 1 || pick.Folder != "f" || pick.Limit != 9 || pick.Text != "" {
		t.Fatalf("a pick block keeps its filters: %+v", pick)
	}
}

func TestStartAndSendingTypes(t *testing.T) {
	for typ, want := range map[string]bool{NodeImport: true, NodePick: true, NodeWait: false, NodeSend: false, NodeLabel: false} {
		if IsStartNode(typ) != want {
			t.Errorf("IsStartNode(%s) = %v", typ, !want)
		}
	}
	for typ, want := range map[string]bool{NodeSend: true, NodeSendText: true, NodeEmail: true, NodeImport: false, NodePick: false, NodeLabel: false, NodeStage: false, NodeWait: false, NodeWaitUntil: false, NodeIfReplied: false} {
		if IsSendingNode(typ) != want {
			t.Errorf("IsSendingNode(%s) = %v", typ, !want)
		}
	}
}
