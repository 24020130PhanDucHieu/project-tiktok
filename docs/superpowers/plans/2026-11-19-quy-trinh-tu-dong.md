# Quy trình tự động kéo thả — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tab "Quy trình": vẽ bằng kéo thả một đồ thị gồm 4 khối (Nhập Excel, Chờ, Gửi chiến dịch, KOC đã trả lời chưa?), bấm bắt đầu một lần, máy chủ tự chạy các nhánh, sống sót qua khởi động lại, không gửi hai lần.

**Architecture:** `WorkflowRunner` (goroutine) giữ các "dấu vị trí" (token) của từng nhánh trong MongoDB, đi tiếp những token đã đến giờ, và điều khiển `ImportRunner` / `CampaignRunner` sẵn có làm bước con (gắn `workflowId/nodeId` để tìm lại sau khi khởi động lại). Frontend dùng React Flow cho khung vẽ; kiểm tra đồ thị ở cả hai phía.

**Tech Stack:** Go + Fiber, MongoDB, React 19 + TS + vitest, `@xyflow/react`.

**Spec:** `docs/superpowers/specs/2026-11-19-quy-trinh-tu-dong-design.md`

## Global Constraints

- Mỗi lúc một quy trình `running` (chỉ mục duy nhất từng phần theo `status`, như `import_jobs`). Chạy tối đa **14 ngày** rồi dừng với lý do `expired`. TTL dọn bản ghi sau 30 ngày.
- Đồ thị: đúng 1 khối `import` (khối bắt đầu, không dây vào); mọi khối khác có **đúng 1 dây vào**; không vòng; mỗi cổng ra ≤ 1 dây; tối đa **15 khối**; `wait` từ **1 phút đến 14 ngày**; mỗi `send` chọn một chiến dịch có ít nhất 1 bước; tổng thời gian chờ dọc mọi nhánh ≤ 14 ngày; tối đa **200 dòng Excel**.
- Chỉ **admin** bắt đầu quy trình có khối `send`; yêu cầu `confirm: true`. `POST /workflows` giới hạn 10 lần/giờ, `PUT /:id/stop` 60 lần/giờ.
- `send` luôn trong khung 08:30–20:30 giờ VN (`IsWithinAllowedSendWindow`), không có tùy chọn bỏ qua; mỗi lượt ≤ `MaxRunKocs` = 50 KOC; hạn mức ngày 30 KOC và 600 tin vẫn do `CampaignRunner` áp dụng.
- Quy trình **bật** công tắc gửi hàng loạt ngay trước mỗi lượt `send` (`BulkSwitch.SetEnabled(ctx, true, "workflow")`); `CampaignRunner` vẫn tự tắt nó khi lượt kết thúc.
- Không bao giờ bắt đầu bước con hai lần: ghi mã bước con vào token **trước** khi coi là đã bắt đầu, và khi `Resume` tìm bước con theo `workflowId/nodeId` trước khi bắt đầu mới.
- Mọi lần bắt đầu, dừng, kết thúc ghi nhật ký kiểm toán (`Auditor.Log`).
- Kiểm tra trước khi xong mỗi task: `cd backend && go vet ./... && go test ./...`; frontend: `cd frontend && npm test && npm run typecheck`. `gofmt -l` chạy trên blob đã stage (máy Windows báo sai do CRLF); chỉ `git add` đúng các file của task.
- Sau khi sửa mã: `graphify update .`.

## Review Focus

1. **Khởi động lại khi một lượt `send` đang chạy** → không bắt đầu lượt thứ hai cho cùng KOC; tìm lại lượt cũ theo `workflowId/nodeId`. (Task 6, Task 8)
2. **Hai nhánh cùng đến khối `send` hoặc `import` một lúc** → chỉ một nhánh bắt đầu, nhánh kia chờ rồi thử lại; không nhánh nào bị bỏ sót. (Task 4, Task 6)
3. **Lượt gửi dừng vì hạn mức ngày hoặc ngoài khung giờ** → KOC còn lại gửi tiếp vào khung giờ ngày hôm sau, không tạo vòng lặp bắt đầu lượt rỗng liên tục. (Task 6)
4. **`if_replied` không xác định được KOC đã trả lời hay chưa** (không có cuộc trò chuyện, lỗi đọc) → KOC **không** vào nhánh "chưa trả lời" (không nhắc nhầm người đã trả lời). (Task 7)
5. **Dừng khi đang chạy lượt con hoặc đang chờ** → quy trình `stopped`, lượt con dừng, không bắt đầu bước tiếp theo. (Task 4, 5, 6)
6. **Danh sách Excel 200 dòng, nhiều KOC được thêm** → chia lượt ≤ 50 và qua nhiều ngày không vượt 14 ngày; số KOC trong khối khớp tổng. (Task 6)

---

### Task 1: Mô hình dữ liệu và kiểm tra đồ thị

**Files:**
- Create: `backend/internal/domain/workflow.go`, `backend/internal/domain/workflow_test.go`

**Interfaces:**
- Produces (`domain`): hằng `WorkflowRunning|WorkflowDone|WorkflowStopped`; `NodeImport="import"`, `NodeWait="wait"`, `NodeSend="send"`, `NodeIfReplied="if_replied"`; `PortOut="out"`, `PortReplied="replied"`, `PortUnanswered="unanswered"`; trạng thái khối `NodePending|NodeRunning|NodeWaiting|NodeDone|NodeStopped`; `MaxWorkflowNodes=15`, `MaxWorkflowRows=200`, `MaxWaitMinutes=14*24*60`, `MaxWorkflowAge=14*24*time.Hour`.
- Kiểu (tag `bson` và `json` camelCase như `ImportJob`): `WorkflowNode{ID, Type string; X, Y float64; WaitMinutes int; CampaignID, CampaignName string; Steps []CampaignStep}`; `WorkflowEdge{ID, From, Port, To string}`; `TokenKoc{KocID string; SentAt *time.Time}`; `WorkflowToken{NodeID string; Kocs, Done []TokenKoc; ReadyAt time.Time; ChildID string}`; `NodeState{NodeID, Status string; Count int; WaitUntil *time.Time; Reason string}`; `Workflow{ID ID; Name string; Nodes []WorkflowNode; Edges []WorkflowEdge; Rows []ImportRow; Source string; Status, StopReason string; StopRequested bool; By string; Tokens []WorkflowToken; States []NodeState; CreatedAt time.Time; FinishedAt *time.Time}`.
- `func ValidateWorkflowGraph(nodes []WorkflowNode, edges []WorkflowEdge) error` trả `domain.Invalid("…")` với câu tiếng Việt nêu rõ lỗi; `func NextNode(edges []WorkflowEdge, from, port string) (to string, ok bool)`; `func MaxPathWaitMinutes(nodes []WorkflowNode, edges []WorkflowEdge) int`.

- [ ] **Step 1: Viết test (bảng)** `TestValidateWorkflowGraph` trong `workflow_test.go`. Hợp lệ: chuỗi import→wait→send; có `if_replied` với hai cổng nối hai nhánh; `if_replied` chỉ nối một cổng. Không hợp lệ (mỗi dòng một lỗi riêng, so khớp một từ khóa của thông báo): không có `import`; hai `import`; khối bỏ trơ (không đến được); hai dây vào một khối; một cổng ra có hai dây; vòng; dây đến khối không tồn tại; cổng sai loại (`replied` trên `wait`); `wait` 0 phút và 14 ngày + 1 phút; `send` không có `CampaignID`; 16 khối; tổng chờ dọc một nhánh 15 ngày.
- [ ] **Step 2: Chạy, thấy đỏ** — `cd backend && go test ./internal/domain -run TestValidateWorkflowGraph` — Expected: FAIL (biên dịch lỗi vì chưa có kiểu).
- [ ] **Step 3: Cài `workflow.go`** theo Interfaces. Duyệt từ khối `import` theo dây để kiểm tra "đến được", đếm dây vào, phát hiện vòng.
- [ ] **Step 4: Chạy lại** — Expected: PASS.
- [ ] **Step 5: Commit** `git add backend/internal/domain/workflow.go backend/internal/domain/workflow_test.go` → `feat(workflow): the graph model and its validation`.

### Task 2: Đánh dấu bước con và ghi mã KOC của dòng nhập

**Files:**
- Modify: `backend/internal/domain/import_job.go`, `backend/internal/domain/bulk_run.go`, `backend/internal/service/import_runner.go`, `backend/internal/service/campaign_runner.go`, `backend/internal/repository/import_job.go`, `backend/internal/repository/bulk_run.go`, `backend/internal/handler/import_job.go` (interface `ImportStarter` phải có cùng chữ ký biến thể `opts ...service.ImportOptions`, vì một phương thức có tham số biến thể không thỏa interface không có nó; sửa luôn các bản giả của nó trong `handler/*_test.go` và `router/import_job_route_test.go`)
- Test: `backend/internal/service/import_runner_test.go`, `backend/internal/service/campaign_runner_test.go`, `backend/internal/repository/import_job_test.go` (chạy ở CI với Mongo)

**Interfaces:**
- `domain.ImportRow` thêm `KocID string` (`bson:"kocId,omitempty"`); `ImportJob` và `BulkRun` thêm `WorkflowID string`, `NodeID string` (`omitempty`).
- `service.ImportOptions{WorkflowID, NodeID string}`; `ImportRunner.Start(ctx, by, source string, rows []domain.ImportRow, opts ...ImportOptions)` (biến thể, các chỗ gọi cũ không đổi). `service.RunOptions` thêm `WorkflowID, NodeID string`.
- `ImportStore` thêm `SetRowKoc(ctx, id domain.ID, index int, kocID string) error`; runner gọi nó khi kết quả là `ImportAdded` (kèm `res.Koc.ID` dạng hex).
- Repo: `ImportJobRepository.FindByWorkflow(ctx, workflowID, nodeID string) (*domain.ImportJob, error)` và `BulkRunRepository.FindByWorkflow(...)  (*domain.BulkRun, error)` — trả `nil, nil` nếu không có; chọn bản ghi mới nhất.

- [ ] **Step 1: Viết test** `TestImportRecordsTheIdOfTheKocItAdded` (fake store ghi lại `SetRowKoc`: dòng `added` có mã, dòng `duplicate` không có), `TestStartCopiesWorkflowTagsIntoTheJob`, và `TestRunKeepsItsWorkflowTags`.
- [ ] **Step 2: Chạy, thấy đỏ** — `go test ./internal/service -run 'TestImportRecordsTheId|TestStartCopies|TestRunKeepsItsWorkflow'` — Expected: FAIL.
- [ ] **Step 3: Cài đặt** như Interfaces. Test repo `TestFindByWorkflowReturnsTheTaggedJobOrNil` (Mongo) viết cùng lúc.
- [ ] **Step 4: Chạy** `go test ./internal/service ./internal/domain` — Expected: PASS (test repo chỉ chạy trong CI).
- [ ] **Step 5: Commit** các file trên → `feat(workflow): tag the jobs of a workflow and record the id of an added KOC`.

### Task 3: Kho lưu quy trình và chỉ mục

**Files:**
- Create: `backend/internal/repository/workflow.go`, `backend/internal/repository/workflow_test.go` (cần `MONGO_TEST_URI`)
- Modify: `backend/internal/database/mongo.go` (khối chỉ mục `workflows`)

**Interfaces:**
- `WorkflowRepository` (`NewWorkflowRepository(db *mongo.Database)`, collection `workflows`) với: `Create(ctx, wf *domain.Workflow) error` (gán ID, CreatedAt, Status running; trùng chỉ mục → `ErrWorkflowInProgress`), `Running`, `Latest`, `Find(ctx, id string)` (NotFound nếu không có), `SaveTokens(ctx, id domain.ID, tokens []domain.WorkflowToken) error`, `SetState(ctx, id domain.ID, st domain.NodeState) error` (thay hoặc thêm theo `NodeID`), `RequestStop(ctx, id string) (bool, error)`, `StopRequested(ctx, id domain.ID) (bool, error)`, `Finish(ctx, id domain.ID, status, reason string, at time.Time) error`.
- `var ErrWorkflowInProgress = domain.Conflict("Đang có một quy trình chạy. Hãy chờ nó xong hoặc dừng nó.")`.
- Chỉ mục: duy nhất từng phần theo `status` (chỉ khi `running`); TTL 30 ngày trên `createdAt`.

- [ ] **Step 1: Viết test** (theo mẫu `import_job_test.go`): `TestOnlyOneWorkflowRuns`, `TestSetStateReplacesTheStateOfTheSameNode`, `TestRequestStopOnlyReachesARunningWorkflow`, `TestSaveTokensKeepsTheOrderOfTheKocs`.
- [ ] **Step 2: Chạy, thấy đỏ** — `go test ./internal/repository -run Workflow` — Expected: FAIL (biên dịch) cục bộ; ở CI: FAIL.
- [ ] **Step 3: Cài đặt** như Interfaces, dùng đúng khuôn của `repository/import_job.go`; thêm chỉ mục vào `mongo.go`.
- [ ] **Step 4: `go vet ./... && go test ./...`** — Expected: PASS cục bộ (test repo bị bỏ qua không có Mongo); CI chạy thật.
- [ ] **Step 5: Commit** → `feat(workflow): the store of workflows and its indexes`.

### Task 4: Lõi điều phối và khối `wait`

**Files:**
- Create: `backend/internal/service/workflow_runner.go`, `backend/internal/service/workflow_runner_test.go`

**Interfaces:**
- `WorkflowStore` (interface thu hẹp của `WorkflowRepository`: `Create, Running, SaveTokens, SetState, StopRequested, Finish`).
- `NewWorkflowRunner(store WorkflowStore, imports WorkflowImports, runs WorkflowRuns, sw BulkSwitch, replies ReplyChecker) *WorkflowRunner` (các interface `WorkflowImports`, `WorkflowRuns`, `ReplyChecker` được định nghĩa ở Task 5, 6, 7; ở task này dùng bản giả rỗng).
- `(*WorkflowRunner) Start(ctx, by string, wf *domain.Workflow) (*domain.Workflow, error)` — kiểm tra `ValidateWorkflowGraph` + số dòng, tạo token đầu ở khối `import`, `Create`, spawn; trả bản sao. `Resume(ctx)`, `Wait()`.
- Hằng: `workflowTick = 5 * time.Second`. Các hàm thay được trong test: `sleep`, `now`.
- Vòng lặp: mỗi tick, nếu `StopRequested` → dừng tất cả (`finish(stopped, "stopped_by_user")`); nếu quy trình quá `MaxWorkflowAge` → `finish(stopped, "expired")`; duyệt token đã `ReadyAt <= now`, gọi `advance(token)` theo loại khối; không còn token → `finish(done, "")`. Mỗi lần token đổi phải `SaveTokens` và `SetState`.
- `advance` trả `(next []WorkflowToken, hold time.Duration, stop string)`: token mới (đi tiếp theo dây `out`), hoặc giữ lại thêm `hold`, hoặc lý do dừng quy trình.

- [ ] **Step 1: Viết test** với kho giả trong bộ nhớ và đồng hồ giả: `TestWaitHoldsTheTokenUntilItsTimeAndThenMovesOn` (chờ 30 phút: sau 29 phút token còn đó, sau 30 phút sang khối kế; danh sách KOC giữ nguyên), `TestTheWorkflowIsDoneWhenNoTokenIsLeft`, `TestStopEndsTheWorkflowAndKeepsWaitingNodesFromRunning`, `TestAWorkflowOlderThanFourteenDaysExpires`, `TestTwoBranchesWaitAtTheSameTimeWithoutBlockingEachOther` (hai token `wait` 10 và 60 phút: token 10 phút đi tiếp sau 10 phút, không đợi token 60).
- [ ] **Step 2: Chạy, thấy đỏ** — `go test ./internal/service -run 'TestWait|TestTheWorkflowIsDone|TestStopEnds|TestAWorkflowOlder|TestTwoBranches'` — Expected: FAIL.
- [ ] **Step 3: Cài đặt** lõi và khối `wait`; khối chưa có (import/send/if_replied) ở task này trả lỗi "chưa hỗ trợ" cho tới các task sau (chỉ dùng trong test của task).
- [ ] **Step 4: Chạy** `go test ./internal/service` — Expected: PASS.
- [ ] **Step 5: Commit** → `feat(workflow): the runner core and the wait block`.

### Task 5: Khối `import`

**Files:**
- Modify: `backend/internal/service/workflow_runner.go`
- Test: `backend/internal/service/workflow_runner_test.go`

**Interfaces:**
- `WorkflowImports{ Start(ctx, by, source string, rows []domain.ImportRow, opts ...ImportOptions) (*domain.ImportJob, error); FindByWorkflow(ctx, workflowID, nodeID string) (*domain.ImportJob, error); Find(ctx, id string) (*domain.ImportJob, error); RequestStop(ctx, id string) (bool, error) }` (thỏa bởi `*ImportRunner` + `*ImportJobRepository`; gói thành một struct ghép ở `main.go`).
- Hành vi: lần đầu token ở khối `import` → `FindByWorkflow` (đã có thì dùng), nếu chưa thì `Start` với `ImportOptions{WorkflowID, NodeID}` rồi ghi `ChildID` vào token; mỗi lần hỏi `Find`: còn `running` → giữ token `workflowTick`; `done` → danh sách KOC = các dòng `added` có `KocID`, sang khối kế; `stopped` → dừng quy trình với lý do của job (`import_stopped:<reason>`). `ErrImportInProgress` → giữ 1 phút, thử lại tối đa 60 lần rồi dừng `import_busy`. Khi quy trình bị dừng và token đang có `ChildID`, gọi `RequestStop` của job đó.
- `NodeState.Count` của `import` = số KOC `added`.

- [ ] **Step 1: Viết test (bộ chạy giả)**: `TestImportBlockStartsOneJobAndPassesTheAddedKocs` (3 dòng: added, duplicate, added → danh sách 2 KOC theo thứ tự), `TestImportBlockResumesAnExistingJobInsteadOfStartingAnother` (đã có job gắn thẻ → `Start` không được gọi), `TestAStoppedImportStopsTheWorkflowWithItsReason`, `TestImportBlockWaitsWhileAnotherImportRunsAndGivesUpAfterAnHour`, `TestStoppingTheWorkflowAsksTheRunningImportToStop`, `TestAnImportWithNoAddedKocEndsTheBranch` (danh sách rỗng → nhánh kết thúc, ghi `Reason="empty_list"`, quy trình `done`).
- [ ] **Step 2: Chạy, thấy đỏ** — `go test ./internal/service -run TestImportBlock` — Expected: FAIL.
- [ ] **Step 3: Cài đặt** khối `import` theo Interfaces.
- [ ] **Step 4: Chạy** `go test ./internal/service` — Expected: PASS.
- [ ] **Step 5: Commit** → `feat(workflow): the import block`.

### Task 6: Khối `send`

**Files:**
- Modify: `backend/internal/service/workflow_runner.go`
- Test: `backend/internal/service/workflow_runner_test.go`

**Interfaces:**
- `WorkflowRuns{ Start(ctx, by, campaignName string, steps []domain.CampaignStep, targets []domain.RunTarget, opts ...RunOptions) (*domain.BulkRun, error); FindByWorkflow(ctx, workflowID, nodeID string) (*domain.BulkRun, error); Find(ctx, id domain.ID) (*domain.BulkRun, error) }` (thỏa bởi `*CampaignRunner` + `*BulkRunRepository`; thêm `BulkRunRepository.Find` nếu chưa có).
- Hành vi mỗi lượt: chia `token.Kocs` thành lượt ≤ `MaxRunKocs`. Trước `Start`: nếu ngoài khung giờ → giữ token đến `08:30` kế (giờ VN); bật công tắc (`sw.SetEnabled(ctx,true,"workflow")`); `Start` với `RunOptions{WorkflowID, NodeID}`; ghi `ChildID`. `ErrRunInProgress` → giữ 1 phút, tối đa 60 lần rồi dừng `send_busy`. Mỗi lần hỏi `Find`: `running` → giữ; hết lượt → với mỗi target: `sent` hoặc `replied` → `Done` (kèm `SentAt = StartedAt`); `queued` hoặc `not_sent` → quay lại `Kocs` chờ lượt sau; mọi trạng thái khác (`skipped`, `failed`, `partial`, `unknown`) → bỏ khỏi danh sách, đếm vào `NodeState.Reason` dạng `"3 bỏ qua"`. Lượt `stopped` với lý do `daily_koc_limit_reached`, `daily_message_cap_reached` hoặc `outside_0830_2030_window` → giữ token đến `08:30` ngày kế (giờ VN) rồi gửi tiếp phần còn lại; lý do khác (`switch_off`, `zalo_errors`, `zalo_not_ready`, `store_unavailable`, `internal_error`) → dừng quy trình (`send_stopped:<reason>`). Hết `Kocs` → sang khối kế với `Done`.
- Hàm giúp: `nextSendWindowStart(now time.Time) time.Time` (08:30 VN của hôm nay nếu chưa tới, ngược lại của ngày kế).
- Khi quy trình bị dừng và token đang có lượt con: tắt công tắc (`SetEnabled(false)`) để `CampaignRunner` tự dừng.

- [ ] **Step 1: Viết test (bộ chạy giả)**: `TestSendBlockSendsInChunksOfFifty` (120 KOC → 3 lượt 50/50/20 nối tiếp), `TestSendBlockDoesNotStartTwoRunsForTheSameNode` (khởi động lại giữa chừng: `FindByWorkflow` thấy lượt đang chạy → `Start` không được gọi lần hai), `TestSendBlockWaitsForTheNextWindowOutsideOfIt` (22:00 → token giữ đến 08:30 hôm sau), `TestSendBlockContinuesTomorrowAfterTheDailyLimit` (lượt dừng `daily_koc_limit_reached` với 20 KOC `not_sent` → hôm sau gửi đúng 20 KOC đó, không gửi lại 30 KOC đã gửi), `TestSendBlockStopsTheWorkflowOnOtherStopReasons` (bảng các lý do), `TestSendBlockKeepsOnlySentAndRepliedKocsForTheNextBlock`, `TestSendBlockSwitchesTheBulkSwitchOnBeforeEveryRun`, `TestTwoBranchesNeverStartTwoRunsAtOnce` (hai token `send`: cái thứ hai gặp `ErrRunInProgress`, chờ rồi gửi sau khi lượt đầu xong), `TestStoppingTheWorkflowSwitchesTheBulkSwitchOff`, `TestTwoHundredKocsFinishWithinFourteenDays` (200 KOC, hạn mức 30/ngày → xong trong 8 ngày giả lập, quy trình `done`).
- [ ] **Step 2: Chạy, thấy đỏ** — `go test ./internal/service -run 'TestSendBlock|TestTwoBranchesNever|TestStoppingTheWorkflowSwitches|TestTwoHundred'` — Expected: FAIL.
- [ ] **Step 3: Cài đặt** khối `send`; đọc `campaign_runner.go` để dùng đúng tên lý do dừng đang có (`stopReason`).
- [ ] **Step 4: Chạy** `go test ./internal/service` — Expected: PASS.
- [ ] **Step 5: Commit** → `feat(workflow): the send block with chunks and next-day continuation`.

### Task 7: Khối `if_replied` và `RepliedSince`

**Files:**
- Modify: `backend/internal/service/workflow_runner.go`, `backend/internal/service/outreach.go` (hoặc file mới `backend/internal/service/replied.go`)
- Test: `backend/internal/service/workflow_runner_test.go`, `backend/internal/service/replied_test.go`

**Interfaces:**
- `ReplyChecker{ RepliedSince(ctx, kocID string, since time.Time) (replied bool, known bool, err error) }`. `OutreachService.RepliedSince` tìm cuộc trò chuyện của KOC bằng cách tra cứu đang dùng ở `SendSequence`/`OpenKocConversation`, rồi gọi `msgs.InboundSince(ctx, conv.ID, since)`; không tìm thấy cuộc trò chuyện → `known=false`.
- Khối `if_replied`: với mỗi `TokenKoc{KocID, SentAt}`: `SentAt == nil` hoặc `known=false` hoặc lỗi → **không** vào nhánh nào (đếm `unknown`); `replied` → nhánh `replied`; ngược lại nhánh `unanswered`. Sinh tối đa hai token mới theo dây của hai cổng (cổng không nối → nhánh kết thúc). `NodeState.Reason` ghi số mỗi nhóm.

- [ ] **Step 1: Viết test**: `TestIfRepliedSplitsTheListByReply`, `TestAKocWhoseReplyCannotBeKnownGoesToNeitherBranch` (known=false, lỗi, SentAt nil), `TestAnUnconnectedPortEndsItsBranch`, `TestBothBranchesRunIndependently` (nhánh `replied` có `wait` 3 ngày, nhánh `unanswered` có `send`: `send` bắt đầu ngay, không chờ 3 ngày); và `TestRepliedSinceSeesOnlyMessagesAfterTheGivenTime`, `TestRepliedSinceUnknownWhenTheKocHasNoConversation` với kho giả.
- [ ] **Step 2: Chạy, thấy đỏ** — `go test ./internal/service -run 'TestIfReplied|TestAKocWhose|TestAnUnconnected|TestBothBranches|TestRepliedSince'` — Expected: FAIL.
- [ ] **Step 3: Cài đặt** `RepliedSince` và khối `if_replied`.
- [ ] **Step 4: Chạy** `go test ./internal/service` — Expected: PASS.
- [ ] **Step 5: Commit** → `feat(workflow): the if-replied block`.

### Task 8: Khởi động lại và một quy trình trọn vẹn qua bộ chạy thật

**Files:**
- Create: `backend/internal/service/workflow_flow_test.go`
- Modify: `backend/internal/service/workflow_runner.go` (chỉ nếu test lộ lỗi)

**Interfaces:** dùng `ImportRunner` thật + `CampaignRunner` thật với kho giả (theo `9c6654c`, `import_runner_test.go`, `campaign_runner_test.go`) và kho quy trình giả.

- [ ] **Step 1: Viết test**: `TestAWholeWorkflowThroughTheRealRunners` (Excel 4 dòng: 3 `added`, 1 `duplicate` → `wait` → `send` → `wait` → `if_replied`; một KOC trả lời giữa chừng → chỉ KOC còn lại nhận lượt `send` thứ hai ở nhánh `unanswered`); `TestRestartDuringImportDoesNotStartASecondJob`; `TestRestartDuringSendDoesNotSendTwiceToAKoc` (khởi động lại khi KOC đang `sending` → `unknown` theo `Resume` sẵn có của `CampaignRunner`, quy trình `Resume` chờ lượt đó và KOC `unknown` không nhận thêm tin); `TestRestartWhileWaitingKeepsTheRemainingWait`.
- [ ] **Step 2: Chạy** `go test ./internal/service -run 'TestAWholeWorkflow|TestRestart'` — Expected: chạy được; nếu đỏ, sửa lỗi thật ở runner (không sửa test cho khớp), ghi lại lỗi và cách sửa trong thông điệp commit.
- [ ] **Step 3: `go vet ./... && go test ./...`** — Expected: PASS.
- [ ] **Step 4: Commit** → `test(workflow): a whole workflow through the real runners, and restarts`.

### Task 9: API, đường dẫn và nối dây

**Files:**
- Create: `backend/internal/handler/workflow.go`, `backend/internal/handler/workflow_test.go`, `backend/internal/router/workflow_route_test.go`
- Modify: `backend/internal/router/router.go` (nhóm `/workflows`, `Handlers.Workflow`), `backend/cmd/api/main.go` (tạo repo, runner, handler; `workflowRunner.Resume(ctx)` sau `runner.Resume(ctx)`)

**Interfaces:**
- `WorkflowHandler{starter WorkflowStarter; jobs WorkflowReader; campaigns CampaignFinder; kocs ...; audit Auditor}`; `WorkflowStarter.Start(ctx, by string, wf *domain.Workflow) (*domain.Workflow, error)`; `WorkflowReader{Latest, Find, RequestStop}`; `CampaignFinder.Find(ctx, id string) (*domain.Campaign, error)`.
- `POST /workflows` body `{name, nodes, edges, rows, source, confirm}`: kiểm tra `confirm` (400), admin nếu có khối `send` (403), `ValidateWorkflowGraph` (400), số dòng, kiểm tra từng dòng bằng `rowProblem` (dòng sai đánh `RowInvalid`, như `/import-jobs`; không còn dòng hợp lệ → 400), chụp bước của chiến dịch cho mỗi khối `send` (`Steps`, `CampaignName`; chiến dịch không tồn tại hoặc không có bước → 400), `CheckSteps` như `/bulk-runs` (ảnh tồn tại); trả **202** với quy trình. `GET /workflows/latest` → `{workflow}`; `GET /workflows/:id`; `PUT /workflows/:id/stop` (409 nếu đã xong) — mọi vai trò đã đăng nhập được xem; chỉ admin được dừng.
- Giới hạn lượt gọi: `POST` 10/giờ, `stop` 60/giờ. Kiểm toán: `workflow.start` (số khối, số dòng), `workflow.stop`.

- [ ] **Step 1: Viết test** (theo `import_job_route_test.go`): `TestStartWorkflowNeedsConfirm`, `TestOnlyAnAdminStartsAWorkflowWithASendBlock`, `TestAMemberMayStartNothingThatSends` (khối `send` bị từ chối 403; đồ thị chỉ `import`+`wait` thì được), `TestAnInvalidGraphIsRefused`, `TestTheStepsOfTheCampaignAreCopiedIntoTheSendNode`, `TestInvalidRowsAreKeptAsInvalidAndOthersRun`, `TestOneWorkflowAtATimeAnswersConflict`, `TestStopNeedsAnAdminAndRefusesAnEndedWorkflow`, `TestWorkflowRoutesAreRateLimited`, `TestWorkflowRequestsNeedLogin`.
- [ ] **Step 2: Chạy, thấy đỏ** — `go test ./internal/handler ./internal/router -run Workflow` — Expected: FAIL.
- [ ] **Step 3: Cài đặt** handler, đường dẫn, nối dây trong `main.go`.
- [ ] **Step 4: `go vet ./... && go test ./...`; kiểm tra gofmt trên blob đã stage** — Expected: PASS.
- [ ] **Step 5: Commit** → `feat(workflow): the API and its wiring`.

### Task 10: Thư viện khung vẽ và logic đồ thị phía trình duyệt

**Files:**
- Modify: `frontend/package.json`, `frontend/package-lock.json`
- Create: `frontend/src/types/workflow.ts`, `frontend/src/features/workflows/graph.ts`, `frontend/src/features/workflows/graph.test.ts`

**Interfaces:**
- Cài `@xyflow/react` (chọn bản mới nhất hỗ trợ React 19; ghi số bản vào thông điệp commit). Nếu không tương thích React 19, **dừng và báo lại** (không đổi thư viện tự ý).
- `types/workflow.ts`: `WorkflowNode`, `WorkflowEdge`, `Workflow`, `NodeState` theo JSON của API (camelCase, giống Task 1).
- `graph.ts`: `validateGraph(nodes, edges): { ok: boolean; problems: { nodeId?: string; message: string }[] }` (cùng luật Task 1: số khối, một import, một dây vào, không vòng, cổng ra ≤ 1 dây, `wait` 1 phút đến 14 ngày, `send` có chiến dịch, tổng chờ ≤ 14 ngày); `stateLabel(state)`, `waitLeftText(untilIso, nowMs)`; `emptyDraft()`; `loadDraft()/saveDraft()` bọc `localStorage` bằng try/catch.

- [ ] **Step 1: `cd frontend && npm install @xyflow/react`**, rồi tạo một test khói `graph.test.ts` import thử `@xyflow/react` và `react` — Expected: `npm run typecheck` sạch với React 19.
- [ ] **Step 2: Viết test** `validateGraph` (cùng bảng trường hợp với Task 1, thông báo tiếng Việt), `waitLeftText` (còn 90 phút → "còn 1 giờ 30 phút"; đã qua → "sắp tới"), `loadDraft` khi `localStorage` ném lỗi → trả bản trống.
- [ ] **Step 3: Chạy, thấy đỏ** — `npm test -- graph` — Expected: FAIL.
- [ ] **Step 4: Cài đặt** `graph.ts`, `types/workflow.ts`.
- [ ] **Step 5: `npm test && npm run typecheck && npm run build`** — Expected: PASS.
- [ ] **Step 6: Commit** → `feat(workflow): the canvas library and the graph checks in the browser`.

### Task 11: Trình vẽ (khung vẽ, khối, bảng cài đặt)

**Files:**
- Create: `frontend/src/features/workflows/WorkflowEditor.tsx`, `WorkflowNodeCard.tsx`, `NodeSettings.tsx`, `workflows.css`
- Test: kiểm tra bằng Playwright (Step 4)

**Interfaces:**
- `WorkflowEditor` props: `{ campaigns: Campaign[]; onStart(payload): Promise<void>; disabled: boolean; isAdmin: boolean }`. Bên trái: bốn khối kéo thả (HTML5 drag onto the canvas → `screenToFlowPosition`); giữa: `<ReactFlow>` với lưới, `Controls`, `MiniMap`, dây nối bằng kéo cổng; phải: `NodeSettings` cho khối đang chọn. Khối `if_replied` có hai cổng ra gắn nhãn "đã trả lời" / "chưa trả lời"; mỗi khối hiện `problems` của `validateGraph` bằng viền cảnh báo; nút **Bắt đầu** khóa khi `!ok` hoặc không phải admin mà có khối `send` (kèm lý do).
- Cài đặt từng khối: `import` — chọn file Excel, dùng lại `parseSheet`/`startRequest` của `kocs/bulkImport.ts` (không viết lại), hiện số dòng hợp lệ; `wait` — số và đơn vị (phút, giờ, ngày); `send` — chọn chiến dịch từ danh sách; `if_replied` — không có cài đặt.
- Bản nháp lưu qua `saveDraft` mỗi khi đồ thị đổi (trừ dòng Excel); tải lại trang thì khôi phục.
- Giao diện theo phong cách các cửa sổ hiện có (lớp `bm-*`, `ti-switch`, màu và cỡ chữ của `styles.css`); chỉ cho vẽ ở màn hình rộng ≥ 900 px, nhỏ hơn thì hiện thông báo "Hãy dùng máy tính để vẽ quy trình".

- [ ] **Step 1: Cài đặt** các component và `workflows.css`.
- [ ] **Step 2: `npm run typecheck && npm test && npm run build`** — Expected: PASS.
- [ ] **Step 3: Chạy giao diện** (`npm run dev`, dùng `mcp__playwright__*`): kéo bốn khối ra, nối import→wait→send→if_replied, thấy viền cảnh báo khi thiếu chiến dịch rồi mất khi chọn, nút Bắt đầu mở khóa; tải lại trang thấy bản nháp còn. Chụp ảnh màn hình.
- [ ] **Step 4: Commit** → `feat(workflow): the drag-and-drop editor`.

### Task 12: Chế độ theo dõi, tab và luồng bắt đầu

**Files:**
- Create: `frontend/src/features/workflows/WorkflowPage.tsx`, `WorkflowRunView.tsx`, `useWorkflowRun.ts`
- Modify: `frontend/src/layouts/SidebarLayout.tsx` (mục "Quy trình"), `frontend/src/App.tsx` (nhánh `selectedNav === 'Quy trình'`, `lazy`)

**Interfaces:**
- `useWorkflowRun()`: gọi `GET /workflows/latest` khi mở tab, nếu `running` thì hỏi `GET /workflows/:id` mỗi 5 giây; dừng hỏi khi quy trình xong; trả `{workflow, stop(), refreshing}`.
- `WorkflowPage`: nếu có quy trình `running` hiển thị `WorkflowRunView` (cùng khung vẽ, **chỉ đọc**, mỗi khối tô màu theo `NodeState.Status`, hiện số KOC và `waitLeftText`; nút **Dừng** cho admin; mô tả lý do khi quy trình dừng bằng bảng tên lý do → câu tiếng Việt, ví dụ `stopped_by_user`, `expired`, `import_stopped:*`, `send_stopped:*`, `send_busy`, `import_busy`); ngược lại hiển thị `WorkflowEditor`. Quy trình vừa kết thúc hiển thị bản tóm tắt một lần (theo mẫu `seenJobId/markJobSeen` của nhập Excel). Bắt đầu: hộp xác nhận ghi "Quy trình này sẽ tự gửi tin cho tối đa N KOC qua M lượt gửi"; gọi `POST /workflows` với `confirm: true`.
- Điện thoại (< 900 px): chỉ có `WorkflowRunView`.

- [ ] **Step 1: Viết test** cho bảng tên lý do và `waitLeftText` mở rộng (`workflowStopText(reason)` trong `graph.ts` + test: mọi lý do trong spec có câu tiếng Việt, lý do lạ trả câu chung).
- [ ] **Step 2: Chạy, thấy đỏ** — `npm test -- graph` — Expected: FAIL; rồi cài đặt, chạy lại: PASS.
- [ ] **Step 3: Cài đặt** trang, chế độ theo dõi, tab, `App.tsx`.
- [ ] **Step 4: `npm run typecheck && npm test && npm run build`** — Expected: PASS.
- [ ] **Step 5: Chạy giao diện với API giả** (Playwright mock `/api/v1/workflows/*`): bắt đầu → khung chuyển chỉ đọc, khối đổi màu khi mock đổi trạng thái, nút Dừng gọi `PUT /:id/stop`, tóm tắt hiện một lần. Chụp ảnh.
- [ ] **Step 6: Commit** → `feat(workflow): the run view, the tab and the start flow`.

### Task 13: Tài liệu và kiểm tra cuối

**Files:**
- Modify: `README.md` (mục Quy trình), tài liệu liên quan trong `docs/` nếu mô tả danh sách tab/API
- Update: `.superpowers/sdd/2026-11-19-quy-trinh-tu-dong/progress.md` (sổ ghi)

- [ ] **Step 1:** Ghi README: tab Quy trình, 4 khối, luật an toàn, giới hạn (một quy trình một lúc, 14 ngày, chỉ admin, chỉ máy tính để vẽ).
- [ ] **Step 2: `cd backend && go vet ./... && go test ./...`; `cd frontend && npm test && npm run typecheck && npm run build`; `graphify update .`** — Expected: tất cả PASS.
- [ ] **Step 3:** Kiểm tra `gofmt` trên blob đã stage của mọi file Go đã đổi.
- [ ] **Step 4: Commit** → `docs(workflow): the Quy trình tab`. Không đẩy lên main nếu chưa được duyệt.
