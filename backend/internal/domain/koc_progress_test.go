package domain

import "testing"

func TestProgressSaysWhereTheKocIsWhenNothingWaitsOnTheOwner(t *testing.T) {
	cases := []struct {
		name string
		k    Koc
		want string
	}{
		{"sample on its way, collaboration still at the sample step", Koc{CollabStep: 2, TrackingCode: "841328496973", Stage: StageShipping}, "Đang giao mẫu"},
		{"sample delivered while the collaboration is at the sample step", Koc{CollabStep: 2, TrackingCode: "8413", Stage: StageDelivered}, "Chờ đăng bài"},
		{"sample step with nothing shipped", Koc{CollabStep: 2}, "Duyệt & gửi mẫu"},
		{"waiting for the video", Koc{CollabStep: 3, Stage: StageDelivered}, "Chờ đăng bài"},
		{"video posted", Koc{CollabStep: 3, Stage: StageVideoPosted}, "Đã lên bài"},
		{"result step", Koc{CollabStep: 4}, "Đã có kết quả"},
		{"completed", Koc{CollabStep: 4, Stage: StageCompleted}, "Hoàn thành"},
		{"before the collaboration starts", Koc{CollabStep: 0, Stage: StageReplied}, ""},
		{"declined", Koc{CollabStep: 2, Stage: StageDeclined}, ""},
		{"archived", Koc{CollabStep: 3, Status: KocArchived}, ""},
		{"do not contact", Koc{CollabStep: 3, Dnc: true}, ""},
	}
	for _, c := range cases {
		if got := c.k.Progress(); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
