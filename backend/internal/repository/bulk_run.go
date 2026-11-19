package repository

import (
	"context"
	"errors"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// BulkRunRepository keeps the bulk runs ("bulk_runs"). A unique index on the running status lets only one run run.
type BulkRunRepository struct{ coll *mongo.Collection }

// NewBulkRunRepository builds the repository.
func NewBulkRunRepository(db *mongo.Database) *BulkRunRepository {
	return &BulkRunRepository{coll: db.Collection("bulk_runs")}
}

// ErrRunInProgress is what Create answers while another run is running.
var ErrRunInProgress = domain.Conflict("Đang có một đợt gửi chạy. Hãy chờ đợt đó xong hoặc dừng nó.")

// Create stores a new running run and sets its ID and CreatedAt.
func (r *BulkRunRepository) Create(ctx context.Context, run *domain.BulkRun) error {
	run.ID, run.CreatedAt, run.Status = domain.NewID(), time.Now(), domain.RunRunning
	if _, err := r.coll.InsertOne(ctx, run); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrRunInProgress
		}
		return err
	}
	return nil
}

// Running returns the run that is running, or nil.
func (r *BulkRunRepository) Running(ctx context.Context) (*domain.BulkRun, error) {
	var run domain.BulkRun
	err := r.coll.FindOne(ctx, bson.M{"status": domain.RunRunning}).Decode(&run)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// FindByWorkflow returns the newest run a block of a workflow started, or nil when there is none.
func (r *BulkRunRepository) FindByWorkflow(ctx context.Context, workflowID, nodeID string) (*domain.BulkRun, error) {
	var run domain.BulkRun
	err := r.coll.FindOne(ctx, bson.M{"workflowId": workflowID, "nodeId": nodeID}, options.FindOne().SetSort(bson.D{{Key: "createdAt", Value: -1}})).Decode(&run)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// Recent returns the newest runs first.
func (r *BulkRunRepository) Recent(ctx context.Context, limit int64) ([]domain.BulkRun, error) {
	cur, err := r.coll.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	out := []domain.BulkRun{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SetTarget replaces the target at index.
func (r *BulkRunRepository) SetTarget(ctx context.Context, runID domain.ID, index int, t domain.RunTarget) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": runID}, bson.M{"$set": bson.M{"targets." + strconv.Itoa(index): t}})
	return err
}

// SetNextAt records when the next KOC will be started (nil clears it).
func (r *BulkRunRepository) SetNextAt(ctx context.Context, runID domain.ID, at *time.Time) error {
	update := bson.M{"$set": bson.M{"nextAt": at}}
	if at == nil {
		update = bson.M{"$unset": bson.M{"nextAt": ""}}
	}
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": runID}, update)
	return err
}

// Finish ends the run; the targets still queued become not_sent with the reason.
func (r *BulkRunRepository) Finish(ctx context.Context, runID domain.ID, status, reason string, at time.Time) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": runID},
		bson.M{
			"$set":   bson.M{"status": status, "stopReason": reason, "finishedAt": at, "targets.$[q].status": domain.TargetNotSent, "targets.$[q].reason": reason},
			"$unset": bson.M{"nextAt": ""},
		},
		options.Update().SetArrayFilters(options.ArrayFilters{Filters: []any{bson.M{"q.status": domain.TargetQueued}}}))
	return err
}

// StartedSince counts the KOCs whose chain was started since the given time, and the messages sent to them.
func (r *BulkRunRepository) StartedSince(ctx context.Context, since time.Time) (kocs, messages int, err error) {
	cur, err := r.coll.Find(ctx, bson.M{"targets.startedAt": bson.M{"$gte": since}})
	if err != nil {
		return 0, 0, err
	}
	var runs []domain.BulkRun
	if err := cur.All(ctx, &runs); err != nil {
		return 0, 0, err
	}
	for _, run := range runs {
		for _, t := range run.Targets {
			if t.StartedAt != nil && !t.StartedAt.Before(since) {
				kocs++
				messages += t.StepsSent
			}
		}
	}
	return kocs, messages, nil
}
