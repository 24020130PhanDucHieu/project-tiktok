package domain

import (
	"strings"
	"testing"
)

func TestValidateTemplateAcceptsADraftThatCannotRunYet(t *testing.T) {
	// a block with no settings yet, a block that is not wired: fine for a draft that is being saved
	nodes := []WorkflowNode{
		{ID: "import-1", Type: NodeImport, X: 40, Y: 120},
		{ID: "send-1", Type: NodeSend, X: 300, Y: 120}, // no campaign chosen yet
		{ID: "email-1", Type: NodeEmail, X: 560, Y: 120},
	}
	edges := []WorkflowEdge{{ID: "e1", From: "import-1", Port: PortOut, To: "send-1"}}
	got, err := ValidateTemplate("  Nhắc KOC  ", nodes, edges)
	if err != nil {
		t.Fatalf("a draft is saved as it is: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("%d nodes", len(got))
	}
	if ValidateWorkflowGraph(nodes, edges) == nil {
		t.Fatal("the same graph is NOT valid to run: saving checks the structure only")
	}
}

func TestValidateTemplateDropsWhatTheBrowserMayNotSendAndWhatEachTypeDoesNotUse(t *testing.T) {
	nodes := []WorkflowNode{
		{ID: "s", Type: NodeSend, CampaignID: "c1", CampaignName: "Của trình duyệt", Steps: []CampaignStep{{Kind: StepText, Text: "nội dung của chiến dịch"}}, Text: "x"},
		{ID: "w", Type: NodeWait, WaitMinutes: 30, LabelID: "l"},
	}
	got, err := ValidateTemplate("Mẫu", nodes, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].CampaignID != "c1" || got[0].CampaignName != "" || got[0].Steps != nil || got[0].Text != "" {
		t.Fatalf("a saved template never holds the steps or the name of a campaign, only its id: %+v", got[0])
	}
	if got[1].WaitMinutes != 30 || got[1].LabelID != "" {
		t.Fatalf("%+v", got[1])
	}
}

func TestValidateTemplateRefuses(t *testing.T) {
	ok := []WorkflowNode{{ID: "a", Type: NodeImport}}
	many := make([]WorkflowNode, MaxWorkflowNodes+1)
	for i := range many {
		many[i] = WorkflowNode{ID: "n" + string(rune('a'+i)), Type: NodeWait, WaitMinutes: 1}
	}
	cases := []struct {
		name  string
		title string
		nodes []WorkflowNode
		edges []WorkflowEdge
		want  string
	}{
		{"no name", "   ", ok, nil, "tên"},
		{"a name that is too long", strings.Repeat("a", 101), ok, nil, "tên"},
		{"no block", "x", nil, nil, "khối"},
		{"too many blocks", "x", many, nil, "tối đa 25"},
		{"two blocks with the same id", "x", []WorkflowNode{{ID: "a", Type: NodeImport}, {ID: "a", Type: NodeWait}}, nil, "trùng"},
		{"a block with no id", "x", []WorkflowNode{{Type: NodeImport}}, nil, "mã"},
		{"an unknown type", "x", []WorkflowNode{{ID: "a", Type: "sms"}}, nil, "loại khối"},
		{"a wire to a block that does not exist", "x", ok, []WorkflowEdge{{ID: "e", From: "a", Port: PortOut, To: "zz"}}, "không tồn tại"},
		{"a port the block does not have", "x", ok, []WorkflowEdge{{ID: "e", From: "a", Port: PortReplied, To: "a"}}, "cổng"},
		{"too many wires", "x", ok, make([]WorkflowEdge, 61), "dây"},
		{"a text that is too long", "x", []WorkflowNode{{ID: "a", Type: NodeSendText, Text: strings.Repeat("a", MaxNodeText+1)}}, nil, "quá dài"},
		{"a subject that is too long", "x", []WorkflowNode{{ID: "a", Type: NodeEmail, Subject: strings.Repeat("a", MaxEmailSubject+1)}}, nil, "quá dài"},
		{"a wait that is too long", "x", []WorkflowNode{{ID: "a", Type: NodeWait, WaitMinutes: MaxWaitMinutes + 1}}, nil, "không hợp lệ"},
		{"a time of day that does not exist", "x", []WorkflowNode{{ID: "a", Type: NodeWaitUntil, AtMinute: 1440}}, nil, "không hợp lệ"},
		{"a pick with too many stages", "x", []WorkflowNode{{ID: "a", Type: NodePick, Stages: []string{"A", "B", "C", "D", "E", "F"}}}, nil, "không hợp lệ"},
		{"a pick with a huge limit", "x", []WorkflowNode{{ID: "a", Type: NodePick, Limit: 100000}}, nil, "không hợp lệ"},
		{"a block placed absurdly far away", "x", []WorkflowNode{{ID: "a", Type: NodeImport, X: 1e12}}, nil, "không hợp lệ"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateTemplate(tc.title, tc.nodes, tc.edges)
			if err == nil {
				t.Fatalf("a bad template was accepted (want a message with %q)", tc.want)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("message %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}
