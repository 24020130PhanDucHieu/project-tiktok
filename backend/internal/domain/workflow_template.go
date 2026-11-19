package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Limits of the saved workflows.
const (
	MaxWorkflowTemplates = 50
	MaxTemplateName      = 100
	maxTemplateEdges     = 60
	maxCoordinate        = 100000
)

// WorkflowTemplate is a workflow drawn and saved to be opened again. It holds the blocks and the wires only: never the steps
// of a campaign (a send block keeps the id of its campaign and takes the steps when it runs) and never a sheet.
type WorkflowTemplate struct {
	ID        ID             `bson:"_id,omitempty" json:"_id"`
	Name      string         `bson:"name" json:"name"`
	Nodes     []WorkflowNode `bson:"nodes" json:"nodes"`
	Edges     []WorkflowEdge `bson:"edges" json:"edges"`
	CreatedBy string         `bson:"createdBy" json:"createdBy"`
	CreatedAt time.Time      `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time      `bson:"updatedAt" json:"updatedAt"`
}

// ValidateTemplate checks that a drawing can be kept: its structure and its sizes, not that it is ready to run (a draft with
// a block that has no settings yet is saved as it is). It returns the blocks cleaned with SanitizeNode.
func ValidateTemplate(name string, nodes []WorkflowNode, edges []WorkflowEdge) ([]WorkflowNode, error) {
	if n := utf8.RuneCountInString(strings.TrimSpace(name)); n < 1 || n > MaxTemplateName {
		return nil, Invalid(fmt.Sprintf("Tên quy trình phải từ 1 đến %d ký tự.", MaxTemplateName))
	}
	if len(nodes) < 1 {
		return nil, Invalid("Quy trình phải có ít nhất một khối.")
	}
	if len(nodes) > MaxWorkflowNodes {
		return nil, Invalid(fmt.Sprintf("Quy trình có tối đa %d khối.", MaxWorkflowNodes))
	}
	byID := map[string]WorkflowNode{}
	clean := make([]WorkflowNode, 0, len(nodes))
	for _, n := range nodes {
		if n.ID == "" || utf8.RuneCountInString(n.ID) > 40 {
			return nil, Invalid("Có khối không có mã hoặc mã quá dài.")
		}
		if _, dup := byID[n.ID]; dup {
			return nil, Invalid("Có hai khối trùng mã.")
		}
		if portsOf(n.Type) == nil {
			return nil, Invalid(fmt.Sprintf("Khối %q có loại khối không hợp lệ.", n.ID))
		}
		if utf8.RuneCountInString(n.Text) > MaxNodeText || utf8.RuneCountInString(n.Subject) > MaxEmailSubject {
			return nil, Invalid(fmt.Sprintf("Nội dung của khối %q quá dài.", n.ID))
		}
		if n.WaitMinutes < 0 || n.WaitMinutes > MaxWaitMinutes || n.AtMinute < 0 || n.AtMinute > 1439 ||
			n.Limit < 0 || n.Limit > MaxPickKocs || len(n.Stages) > MaxPickStages || utf8.RuneCountInString(n.Folder) > 80 ||
			n.X < -maxCoordinate || n.X > maxCoordinate || n.Y < -maxCoordinate || n.Y > maxCoordinate {
			return nil, Invalid(fmt.Sprintf("Cài đặt của khối %q không hợp lệ.", n.ID))
		}
		for _, s := range n.Stages {
			if utf8.RuneCountInString(s) > 40 {
				return nil, Invalid(fmt.Sprintf("Cài đặt của khối %q không hợp lệ.", n.ID))
			}
		}
		byID[n.ID] = n
		clean = append(clean, SanitizeNode(n))
	}
	if len(edges) > maxTemplateEdges {
		return nil, Invalid(fmt.Sprintf("Quy trình có tối đa %d dây nối.", maxTemplateEdges))
	}
	for _, e := range edges {
		from, okFrom := byID[e.From]
		_, okTo := byID[e.To]
		if !okFrom || !okTo {
			return nil, Invalid("Có dây nối tới một khối không tồn tại.")
		}
		if !contains(portsOf(from.Type), e.Port) {
			return nil, Invalid(fmt.Sprintf("Khối %q không có cổng ra %q.", e.From, e.Port))
		}
	}
	return clean, nil
}
