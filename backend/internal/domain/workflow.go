package domain

import (
	"fmt"
	"regexp"
	"time"
	"unicode/utf8"
)

// Status of a workflow.
const (
	WorkflowRunning = "running"
	WorkflowDone    = "done"
	WorkflowStopped = "stopped"
)

// Type of a block on the canvas.
const (
	NodeImport    = "import"     // starts a workflow: imports the rows of an Excel sheet
	NodePick      = "pick"       // starts a workflow: takes KOCs that are already in the directory, by stage and folder
	NodeWait      = "wait"       // waits for a while
	NodeWaitUntil = "wait_until" // waits until a clock time (Vietnam time)
	NodeSend      = "send"       // sends a saved campaign to the KOCs it holds
	NodeSendText  = "send_text"  // sends one text on Zalo
	NodeEmail     = "email"      // sends an email
	NodeLabel     = "label"      // puts a label on the conversation of every KOC
	NodeStage     = "stage"      // moves every KOC to a stage
	NodeIfReplied = "if_replied" // splits the KOCs into the ones who answered and the ones who did not
)

// Output port of a block. Every block has "out" except if_replied, which has two.
const (
	PortOut        = "out"
	PortReplied    = "replied"
	PortUnanswered = "unanswered"
)

// Status of one block while the workflow runs.
const (
	NodePending = "pending"
	NodeRunning = "running"
	NodeWaiting = "waiting"
	NodeDone    = "done"
	NodeStopped = "stopped"
)

// Limits of a workflow.
const (
	MaxWorkflowNodes = 25
	MaxWorkflowRows  = 200
	MaxWaitMinutes   = 14 * 24 * 60
	MaxWorkflowAge   = 14 * 24 * time.Hour
	MaxNodeText      = 2000 // characters of a text or an email
	MaxEmailSubject  = 150
	MaxPickKocs      = 200
	MaxPickStages    = 5
)

// AllowedStageTargets are the stages a stage block may move KOCs to: the ones that carry no reason code and no side effect.
var AllowedStageTargets = []string{StageContacted, StageNoReply, StageReplied, StageDormant, StageStalled}

var labelIDRe = regexp.MustCompile(`^[0-9a-f]{24}$`)

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// IsStartNode tells the block types that start a workflow: a workflow has exactly one.
func IsStartNode(nodeType string) bool { return nodeType == NodeImport || nodeType == NodePick }

// IsSendingNode tells the block types that message people by themselves: a workflow with one needs an admin and a confirmation.
func IsSendingNode(nodeType string) bool {
	return nodeType == NodeSend || nodeType == NodeSendText || nodeType == NodeEmail
}

// WorkflowNode is one block of the canvas. The fields after Y are the settings of the block types that use them.
type WorkflowNode struct {
	ID   string  `bson:"id" json:"id"`
	Type string  `bson:"type" json:"type"`
	X    float64 `bson:"x" json:"x"`
	Y    float64 `bson:"y" json:"y"`

	WaitMinutes  int            `bson:"waitMinutes,omitempty" json:"waitMinutes,omitempty"`   // wait
	AtMinute     int            `bson:"atMinute,omitempty" json:"atMinute,omitempty"`         // wait_until: minutes after midnight, Vietnam time
	CampaignID   string         `bson:"campaignId,omitempty" json:"campaignId,omitempty"`     // send
	CampaignName string         `bson:"campaignName,omitempty" json:"campaignName,omitempty"` // send: filled by the server when the workflow starts
	Steps        []CampaignStep `bson:"steps,omitempty" json:"steps,omitempty"`               // send: a copy of the campaign taken when the workflow starts
	Text         string         `bson:"text,omitempty" json:"text,omitempty"`                 // send_text, email: {ten} and {kenh} are filled per KOC
	Subject      string         `bson:"subject,omitempty" json:"subject,omitempty"`           // email: empty means the default subject
	LabelID      string         `bson:"labelId,omitempty" json:"labelId,omitempty"`           // label
	Stage        string         `bson:"stage,omitempty" json:"stage,omitempty"`               // stage
	Stages       []string       `bson:"stages,omitempty" json:"stages,omitempty"`             // pick
	Folder       string         `bson:"folder,omitempty" json:"folder,omitempty"`             // pick
	Limit        int            `bson:"limit,omitempty" json:"limit,omitempty"`               // pick: how many KOCs at most
}

// SanitizeNode keeps what a block of its type may carry, and nothing else: the browser is not trusted to say which steps a
// send block sends (the server copies them from the saved campaign) or to put the settings of one type on another.
func SanitizeNode(n WorkflowNode) WorkflowNode {
	out := WorkflowNode{ID: n.ID, Type: n.Type, X: n.X, Y: n.Y}
	switch n.Type {
	case NodeWait:
		out.WaitMinutes = n.WaitMinutes
	case NodeWaitUntil:
		out.AtMinute = n.AtMinute
	case NodeSend:
		out.CampaignID = n.CampaignID
	case NodeSendText:
		out.Text = n.Text
	case NodeEmail:
		out.Text, out.Subject = n.Text, n.Subject
	case NodeLabel:
		out.LabelID = n.LabelID
	case NodeStage:
		out.Stage = n.Stage
	case NodePick:
		out.Stages, out.Folder, out.Limit = n.Stages, n.Folder, n.Limit
	}
	return out
}

// WorkflowEdge is a wire from the output port of one block to the input of another.
type WorkflowEdge struct {
	ID   string `bson:"id" json:"id"`
	From string `bson:"from" json:"from"`
	Port string `bson:"port" json:"port"`
	To   string `bson:"to" json:"to"`
}

// TokenKoc is a KOC that travels through the workflow, and when its steps were sent (nil when they were not).
type TokenKoc struct {
	KocID  string     `bson:"kocId" json:"kocId"`
	SentAt *time.Time `bson:"sentAt,omitempty" json:"sentAt,omitempty"`
}

// WorkflowToken is one branch that is running: the block it is at, the KOCs it carries, and when it may go on. Kocs is
// what the block still has to work on; Done is what the block has finished with (a send block fills it chunk by chunk).
type WorkflowToken struct {
	NodeID  string     `bson:"nodeId" json:"nodeId"`
	Kocs    []TokenKoc `bson:"kocs" json:"kocs"`
	Done    []TokenKoc `bson:"done,omitempty" json:"done,omitempty"`
	ReadyAt time.Time  `bson:"readyAt" json:"readyAt"`
	ChildID string     `bson:"childId,omitempty" json:"childId,omitempty"` // the import job or the bulk run this block started
	Waits   int        `bson:"waits,omitempty" json:"waits,omitempty"`     // times the block waited for another job to finish
	Chunk   int        `bson:"chunk,omitempty" json:"chunk,omitempty"`     // send block: the number of the chunk being sent, written BEFORE its run is started
	Skipped int        `bson:"skipped,omitempty" json:"skipped,omitempty"` // send block: KOCs that dropped out (skipped, failed, unknown)
	Reads   int        `bson:"reads,omitempty" json:"reads,omitempty"`     // times a read of the stores failed in a row (the next try is later)
}

// NodeState is what the screen shows on a block.
type NodeState struct {
	NodeID    string     `bson:"nodeId" json:"nodeId"`
	Status    string     `bson:"status" json:"status"`
	Count     int        `bson:"count" json:"count"`
	WaitUntil *time.Time `bson:"waitUntil,omitempty" json:"waitUntil,omitempty"`
	Reason    string     `bson:"reason,omitempty" json:"reason,omitempty"`
}

// Workflow is one press of "Bắt đầu" in the Quy trình tab: the graph, the rows of the sheet, and how far every branch got.
// At most one workflow is running.
type Workflow struct {
	ID            ID              `bson:"_id,omitempty" json:"_id"`
	Name          string          `bson:"name" json:"name"`
	Nodes         []WorkflowNode  `bson:"nodes" json:"nodes"`
	Edges         []WorkflowEdge  `bson:"edges" json:"edges"`
	Rows          []ImportRow     `bson:"rows,omitempty" json:"rows,omitempty"`
	Source        string          `bson:"source" json:"source"`
	Status        string          `bson:"status" json:"status"`
	StopReason    string          `bson:"stopReason,omitempty" json:"stopReason,omitempty"`
	StopRequested bool            `bson:"stopRequested,omitempty" json:"-"`
	By            string          `bson:"by" json:"by"`
	Tokens        []WorkflowToken `bson:"tokens" json:"tokens"`
	States        []NodeState     `bson:"states" json:"states"`
	CreatedAt     time.Time       `bson:"createdAt" json:"createdAt"`
	FinishedAt    *time.Time      `bson:"finishedAt,omitempty" json:"finishedAt,omitempty"`
}

// portsOf is the output ports a block type has, or nil for a type that does not exist.
func portsOf(nodeType string) []string {
	switch nodeType {
	case NodeImport, NodePick, NodeWait, NodeWaitUntil, NodeSend, NodeSendText, NodeEmail, NodeLabel, NodeStage:
		return []string{PortOut}
	case NodeIfReplied:
		return []string{PortReplied, PortUnanswered}
	}
	return nil
}

// NextNode is the block the wire of a port leads to.
func NextNode(edges []WorkflowEdge, from, port string) (string, bool) {
	for _, e := range edges {
		if e.From == from && e.Port == port {
			return e.To, true
		}
	}
	return "", false
}

// waitMinutesOf is how long a block holds a branch, at most: a wait_until holds it for a day at most.
func waitMinutesOf(n WorkflowNode) int {
	switch n.Type {
	case NodeWait:
		return n.WaitMinutes
	case NodeWaitUntil:
		return 24 * 60
	}
	return 0
}

// MaxPathWaitMinutes is the longest time one branch waits, from the first block to the end of the branch.
func MaxPathWaitMinutes(nodes []WorkflowNode, edges []WorkflowEdge) int {
	byID := map[string]WorkflowNode{}
	start := ""
	for _, n := range nodes {
		byID[n.ID] = n
		if IsStartNode(n.Type) {
			start = n.ID
		}
	}
	seen := map[string]bool{}
	var longest func(id string) int
	longest = func(id string) int {
		n, ok := byID[id]
		if !ok || seen[id] { // a missing block or a cycle: stop here (the graph check reports it)
			return 0
		}
		seen[id] = true
		defer delete(seen, id)
		best := 0
		for _, port := range portsOf(n.Type) {
			if to, ok := NextNode(edges, id, port); ok {
				if v := longest(to); v > best {
					best = v
				}
			}
		}
		return waitMinutesOf(n) + best
	}
	return longest(start)
}

func validText(s string) bool {
	n := utf8.RuneCountInString(s)
	return n >= 1 && n <= MaxNodeText
}

// configProblem says what is wrong with the settings of one block, or "" when they are fine.
func configProblem(n WorkflowNode) string {
	switch n.Type {
	case NodeWait:
		if n.WaitMinutes < 1 || n.WaitMinutes > MaxWaitMinutes {
			return "Thời gian chờ phải từ 1 phút đến 14 ngày."
		}
	case NodeWaitUntil:
		if n.AtMinute < 0 || n.AtMinute > 1439 {
			return "Giờ chờ không hợp lệ."
		}
	case NodeSend:
		if n.CampaignID == "" {
			return "Khối Gửi chiến dịch chưa chọn chiến dịch."
		}
	case NodeSendText:
		if !validText(n.Text) {
			return fmt.Sprintf("Khối Gửi tin Zalo lẻ cần nội dung từ 1 đến %d ký tự.", MaxNodeText)
		}
	case NodeEmail:
		if !validText(n.Text) {
			return fmt.Sprintf("Khối Gửi email cần nội dung từ 1 đến %d ký tự.", MaxNodeText)
		}
		if utf8.RuneCountInString(n.Subject) > MaxEmailSubject {
			return fmt.Sprintf("Chủ đề email tối đa %d ký tự.", MaxEmailSubject)
		}
	case NodeLabel:
		if !labelIDRe.MatchString(n.LabelID) {
			return "Khối Gắn nhãn chưa chọn nhãn."
		}
	case NodeStage:
		if !contains(AllowedStageTargets, n.Stage) {
			return "Khối Đổi giai đoạn chưa chọn giai đoạn."
		}
	case NodePick:
		if len(n.Stages) == 0 && n.Folder == "" {
			return "Khối Lấy KOC có sẵn cần chọn giai đoạn hoặc thư mục."
		}
		if len(n.Stages) > MaxPickStages {
			return fmt.Sprintf("Khối Lấy KOC có sẵn chọn tối đa %d giai đoạn.", MaxPickStages)
		}
		for _, s := range n.Stages {
			if !contains(KocStages, s) {
				return "Khối Lấy KOC có sẵn có giai đoạn không hợp lệ."
			}
		}
		if n.Limit < 1 || n.Limit > MaxPickKocs {
			return fmt.Sprintf("Giới hạn số KOC của khối Lấy KOC có sẵn từ 1 đến %d.", MaxPickKocs)
		}
	}
	return ""
}

// ValidateWorkflowGraph says in words what is wrong with a graph, or returns nil. The server checks every graph again:
// the screen checks too, but it is not trusted.
func ValidateWorkflowGraph(nodes []WorkflowNode, edges []WorkflowEdge) error {
	if len(nodes) > MaxWorkflowNodes {
		return Invalid(fmt.Sprintf("Quy trình có tối đa %d khối.", MaxWorkflowNodes))
	}
	byID := map[string]WorkflowNode{}
	starts := 0
	for _, n := range nodes {
		if _, dup := byID[n.ID]; dup || n.ID == "" {
			return Invalid("Có hai khối trùng mã hoặc khối không có mã.")
		}
		byID[n.ID] = n
		if portsOf(n.Type) == nil {
			return Invalid(fmt.Sprintf("Khối %q có loại khối không hợp lệ.", n.ID))
		}
		if IsStartNode(n.Type) {
			starts++
		}
	}
	switch {
	case starts == 0:
		return Invalid("Quy trình chưa có khối bắt đầu (Nhập Excel hoặc Lấy KOC có sẵn).")
	case starts > 1:
		return Invalid("Quy trình chỉ được có một khối bắt đầu.")
	}

	type portKey struct{ from, port string }
	outs := map[portKey]int{}
	ins := map[string]int{}
	for _, e := range edges {
		from, okFrom := byID[e.From]
		_, okTo := byID[e.To]
		if !okFrom || !okTo {
			return Invalid("Có dây nối tới một khối không tồn tại.")
		}
		if !contains(portsOf(from.Type), e.Port) {
			return Invalid(fmt.Sprintf("Khối %q không có cổng ra %q.", e.From, e.Port))
		}
		outs[portKey{e.From, e.Port}]++
		ins[e.To]++
	}
	for k, c := range outs {
		if c > 1 {
			return Invalid(fmt.Sprintf("Cổng %q của khối %q có nhiều hơn một dây ra.", k.port, k.from))
		}
	}
	for id, c := range ins {
		if IsStartNode(byID[id].Type) {
			return Invalid("Khối bắt đầu không nhận dây vào.")
		}
		if c > 1 {
			return Invalid(fmt.Sprintf("Khối %q có nhiều hơn một dây vào.", id))
		}
	}

	for _, n := range nodes {
		if msg := configProblem(n); msg != "" {
			return Invalid(msg)
		}
	}

	// a cycle: follow the wires from every block; coming back to a block that is still being followed is a cycle
	state := map[string]int{} // 0 not seen, 1 being followed, 2 done
	var visit func(id string) bool
	visit = func(id string) bool {
		state[id] = 1
		for _, port := range portsOf(byID[id].Type) {
			to, ok := NextNode(edges, id, port)
			if !ok {
				continue
			}
			if state[to] == 1 || (state[to] == 0 && visit(to)) {
				return true
			}
		}
		state[id] = 2
		return false
	}
	for _, n := range nodes {
		if state[n.ID] == 0 && visit(n.ID) {
			return Invalid("Các khối đang nối thành vòng.")
		}
	}

	// every block must be reached from the start block
	reached := map[string]bool{}
	var walk func(id string)
	walk = func(id string) {
		reached[id] = true
		for _, port := range portsOf(byID[id].Type) {
			if to, ok := NextNode(edges, id, port); ok && !reached[to] {
				walk(to)
			}
		}
	}
	for _, n := range nodes {
		if IsStartNode(n.Type) {
			walk(n.ID)
		}
	}
	for _, n := range nodes {
		if !reached[n.ID] {
			return Invalid(fmt.Sprintf("Khối %q chưa được nối vào quy trình.", n.ID))
		}
	}

	if MaxPathWaitMinutes(nodes, edges) > MaxWaitMinutes {
		return Invalid("Tổng thời gian chờ của một nhánh không được quá 14 ngày.")
	}
	return nil
}
