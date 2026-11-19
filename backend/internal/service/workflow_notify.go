package service

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// WorkflowNotifier tells somebody that a workflow ended (it is not the runner's job to know how).
type WorkflowNotifier interface {
	Notify(ctx context.Context, wf *domain.Workflow)
}

// WithNotifier tells the notifier, once, when a workflow ends (done or stopped).
func (r *WorkflowRunner) WithNotifier(n WorkflowNotifier) *WorkflowRunner {
	r.notifier = n
	return r
}

// WorkflowMailNotifier mails the person who started a workflow when it ends: a workflow can run for days, and nobody should
// have to keep the tab open to learn that it finished or why it stopped.
type WorkflowMailNotifier struct {
	mail SubjectMailer
	url  string
}

// NewWorkflowMailNotifier builds the notifier; url is the address of the app, put at the end of the mail.
func NewWorkflowMailNotifier(mail SubjectMailer, url string) *WorkflowMailNotifier {
	return &WorkflowMailNotifier{mail: mail, url: url}
}

var blockTitles = map[string]string{
	domain.NodeImport: "Nhập Excel", domain.NodePick: "Lấy KOC có sẵn", domain.NodeWait: "Chờ", domain.NodeWaitUntil: "Chờ đến giờ",
	domain.NodeSend: "Gửi chiến dịch", domain.NodeSendText: "Gửi tin Zalo", domain.NodeEmail: "Gửi email",
	domain.NodeLabel: "Gắn nhãn", domain.NodeStage: "Đổi giai đoạn", domain.NodeIfReplied: "KOC đã trả lời chưa?",
}

var stopTexts = map[string]string{
	"stopped_by_user": "Đã dừng theo yêu cầu.",
	"expired":         "Quy trình chạy quá 14 ngày nên đã tự dừng.",
	"import_busy":     "Một đợt nhập khác chạy quá lâu nên quy trình đã dừng.",
	"send_busy":       "Một đợt gửi khác chạy quá lâu nên quy trình đã dừng.",
	"import_failed":   "Không bắt đầu được bước nhập KOC.",
	"send_failed":     "Không bắt đầu được bước gửi tin.",
	"internal_error":  "Máy chủ gặp lỗi nên quy trình đã dừng.",
}

var sendStopTexts = map[string]string{
	"switch_off":        "công tắc Gửi hàng loạt đã bị tắt.",
	"zalo_errors":       "Zalo lỗi nhiều lần liên tiếp.",
	"zalo_not_ready":    "Zalo cá nhân chưa kết nối.",
	"store_unavailable": "máy chủ chưa đọc được dữ liệu.",
	"internal_error":    "máy chủ gặp lỗi.",
}

func stopSentence(reason string) string {
	if t, ok := stopTexts[reason]; ok {
		return t
	}
	if code, ok := strings.CutPrefix(reason, "send_stopped:"); ok {
		why := sendStopTexts[code]
		if why == "" {
			why = "một lý do chưa rõ."
		}
		return "Bước gửi tin đã dừng vì " + why
	}
	if strings.HasPrefix(reason, "import_stopped:") {
		return "Bước nhập KOC đã dừng."
	}
	if reason == "" {
		return "Quy trình đã dừng."
	}
	return fmt.Sprintf("Quy trình đã dừng (%s).", reason)
}

// Notify mails the person who started the workflow. A name that is not an email address (a seeded account) gets nothing,
// and a mail that cannot be sent is only logged: the workflow has ended whatever happens to the mail.
func (m *WorkflowMailNotifier) Notify(_ context.Context, wf *domain.Workflow) {
	if m == nil || m.mail == nil || !strings.Contains(wf.By, "@") {
		return
	}
	subject, text := NotificationText(wf, m.url)
	if err := m.mail.Send(wf.By, subject, text); err != nil {
		log.Printf("workflow %s: cannot mail %s that it ended: %v", wf.ID, wf.By, err)
	}
}

// NotificationText is the subject and the body of the mail about an ended workflow.
func NotificationText(wf *domain.Workflow, url string) (subject, body string) {
	var b strings.Builder
	if wf.Status == domain.WorkflowStopped {
		subject = fmt.Sprintf("Quy trình %q đã dừng", wf.Name)
		b.WriteString(stopSentence(wf.StopReason) + "\n")
	} else {
		subject = fmt.Sprintf("Quy trình %q đã xong", wf.Name)
		b.WriteString("Quy trình đã chạy xong.\n")
	}
	var lines []string
	for _, n := range wf.Nodes {
		for _, s := range wf.States {
			if s.NodeID != n.ID || s.Status == domain.NodePending {
				continue
			}
			line := fmt.Sprintf("- %s: %d KOC", blockTitles[n.Type], s.Count)
			if s.Reason != "" {
				line += " (" + s.Reason + ")"
			}
			lines = append(lines, line)
		}
	}
	if len(lines) > 0 {
		b.WriteString("\nCác bước:\n" + strings.Join(lines, "\n") + "\n")
	}
	if url != "" {
		b.WriteString("\nXem chi tiết (từng KOC, từng bước) ở tab Quy trình > Lịch sử chạy: " + url + "\n")
	}
	return subject, b.String()
}
