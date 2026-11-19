package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// WorkflowRepository keeps the workflows ("workflows"). A unique index on the running status lets only one workflow run.
type WorkflowRepository struct{ coll *mongo.Collection }

// NewWorkflowRepository builds the repository.
func NewWorkflowRepository(db *mongo.Database) *WorkflowRepository {
	return &WorkflowRepository{coll: db.Collection("workflows")}
}

// ErrWorkflowInProgress is what Create answers while another workflow is running.
var ErrWorkflowInProgress = domain.Conflict("Đang có một quy trình chạy. Hãy chờ nó xong hoặc dừng nó.")

var errWorkflowNotFound = domain.NotFound("Không tìm thấy quy trình.")

// Create stores a new running workflow and sets its ID and CreatedAt.
func (r *WorkflowRepository) Create(ctx context.Context, wf *domain.Workflow) error {
	wf.ID, wf.CreatedAt, wf.Status = domain.NewID(), time.Now(), domain.WorkflowRunning
	if wf.Tokens == nil {
		wf.Tokens = []domain.WorkflowToken{} // an empty list, not null: $push and the positional update need an array
	}
	if wf.States == nil {
		wf.States = []domain.NodeState{}
	}
	if _, err := r.coll.InsertOne(ctx, wf); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrWorkflowInProgress
		}
		return err
	}
	return nil
}

func (r *WorkflowRepository) one(ctx context.Context, filter bson.M, opts ...*options.FindOneOptions) (*domain.Workflow, error) {
	var wf domain.Workflow
	err := r.coll.FindOne(ctx, filter, opts...).Decode(&wf)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &wf, nil
}

// Running returns the workflow that is running, or nil.
func (r *WorkflowRepository) Running(ctx context.Context) (*domain.Workflow, error) {
	return r.one(ctx, bson.M{"status": domain.WorkflowRunning})
}

// Latest returns the workflow that was made last (running or ended), or nil when there is none.
func (r *WorkflowRepository) Latest(ctx context.Context) (*domain.Workflow, error) {
	return r.one(ctx, bson.M{}, options.FindOne().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
}

// List returns the newest workflows as summaries: the blocks and their states, without the sheet (people's phones and
// emails), the tokens and the report, which are read one workflow at a time with Find.
func (r *WorkflowRepository) List(ctx context.Context, limit int) ([]*domain.Workflow, error) {
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(int64(limit)).
		SetProjection(bson.M{"rows": 0, "tokens": 0, "log": 0})
	cur, err := r.coll.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []*domain.Workflow
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Find returns one workflow, or a NotFound fault.
func (r *WorkflowRepository) Find(ctx context.Context, id string) (*domain.Workflow, error) {
	oid, err := domain.ParseID(id)
	if err != nil {
		return nil, errWorkflowNotFound
	}
	wf, err := r.one(ctx, bson.M{"_id": oid})
	if err != nil {
		return nil, err
	}
	if wf == nil {
		return nil, errWorkflowNotFound
	}
	return wf, nil
}

// SaveTokens replaces the tokens: the branches that are running, where they are and what they carry.
func (r *WorkflowRepository) SaveTokens(ctx context.Context, id domain.ID, tokens []domain.WorkflowToken) error {
	if tokens == nil {
		tokens = []domain.WorkflowToken{}
	}
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"tokens": tokens}})
	return err
}

// SetState writes what the screen shows on a block: it replaces the state of the same block, or adds one.
func (r *WorkflowRepository) SetState(ctx context.Context, id domain.ID, st domain.NodeState) error {
	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": id, "states.nodeId": st.NodeID}, bson.M{"$set": bson.M{"states.$": st}})
	if err != nil {
		return err
	}
	if res.MatchedCount > 0 {
		return nil
	}
	_, err = r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$push": bson.M{"states": st}})
	return err
}

// AddEvent adds a line to the report of a workflow; only the newest MaxLogEvents lines are kept.
func (r *WorkflowRepository) AddEvent(ctx context.Context, id domain.ID, ev domain.WorkflowEvent) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$push": bson.M{"log": bson.M{"$each": []domain.WorkflowEvent{ev}, "$slice": -domain.MaxLogEvents}}})
	return err
}

// RequestStop asks a running workflow to stop. It reports whether a running workflow was found.
func (r *WorkflowRepository) RequestStop(ctx context.Context, id string) (bool, error) {
	oid, err := domain.ParseID(id)
	if err != nil {
		return false, errWorkflowNotFound
	}
	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": oid, "status": domain.WorkflowRunning}, bson.M{"$set": bson.M{"stopRequested": true}})
	if err != nil {
		return false, err
	}
	return res.MatchedCount > 0, nil
}

// StopRequested reports whether somebody asked the workflow to stop.
func (r *WorkflowRepository) StopRequested(ctx context.Context, id domain.ID) (bool, error) {
	var doc struct {
		StopRequested bool `bson:"stopRequested"`
	}
	err := r.coll.FindOne(ctx, bson.M{"_id": id}, options.FindOne().SetProjection(bson.M{"stopRequested": 1})).Decode(&doc)
	if err != nil {
		return false, err
	}
	return doc.StopRequested, nil
}

// Finish ends the workflow and clears the tokens: nothing is running any more.
func (r *WorkflowRepository) Finish(ctx context.Context, id domain.ID, status, reason string, at time.Time) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set": bson.M{"status": status, "stopReason": reason, "finishedAt": at, "tokens": []domain.WorkflowToken{}},
	})
	return err
}
