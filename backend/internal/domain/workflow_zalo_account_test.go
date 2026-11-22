package domain

import "testing"

func TestASendBlockKeepsItsZaloAccount(t *testing.T) {
	for _, typ := range []string{NodeSend, NodeSendText} {
		got := SanitizeNode(WorkflowNode{ID: "n", Type: typ, CampaignID: "c", Text: "hi", ZaloAccountID: "acc-B"})
		if got.ZaloAccountID != "acc-B" {
			t.Fatalf("%s lost its account: %+v", typ, got)
		}
	}
}

func TestOtherBlocksCannotCarryAZaloAccount(t *testing.T) {
	// the browser is not trusted to put the settings of one block type on another
	for _, typ := range []string{NodeWait, NodeWaitUntil, NodeLabel, NodeStage, NodeIfReplied, NodePick, NodeImport, NodeEmail} {
		got := SanitizeNode(WorkflowNode{ID: "n", Type: typ, ZaloAccountID: "acc-B"})
		if got.ZaloAccountID != "" {
			t.Fatalf("a %s block kept an account: %+v", typ, got)
		}
	}
}
