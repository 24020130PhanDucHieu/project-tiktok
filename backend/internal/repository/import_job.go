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

// ImportJobRepository keeps the import jobs ("import_jobs"). A unique index on the running status lets only one job run.
type ImportJobRepository struct{ coll *mongo.Collection }

// NewImportJobRepository builds the repository.
func NewImportJobRepository(db *mongo.Database) *ImportJobRepository {
	return &ImportJobRepository{coll: db.Collection("import_jobs")}
}

// ErrImportInProgress is what Create answers while another import job is running.
var ErrImportInProgress = domain.Conflict("Đang có một đợt nhập KOC chạy. Hãy chờ đợt đó xong hoặc dừng nó.")

// Create stores a new running job and sets its ID and CreatedAt.
func (r *ImportJobRepository) Create(ctx context.Context, job *domain.ImportJob) error {
	job.ID, job.CreatedAt, job.Status = domain.NewID(), time.Now(), domain.ImportRunning
	if _, err := r.coll.InsertOne(ctx, job); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrImportInProgress
		}
		return err
	}
	return nil
}

// Running returns the job that is running, or nil.
func (r *ImportJobRepository) Running(ctx context.Context) (*domain.ImportJob, error) {
	var job domain.ImportJob
	err := r.coll.FindOne(ctx, bson.M{"status": domain.ImportRunning}).Decode(&job)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// Find returns one job, or a NotFound fault.
func (r *ImportJobRepository) Find(ctx context.Context, id string) (*domain.ImportJob, error) {
	oid, err := domain.ParseID(id)
	if err != nil {
		return nil, domain.NotFound("Không tìm thấy đợt nhập.")
	}
	var job domain.ImportJob
	if err := r.coll.FindOne(ctx, bson.M{"_id": oid}).Decode(&job); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.NotFound("Không tìm thấy đợt nhập.")
		}
		return nil, err
	}
	return &job, nil
}

// Recent returns the newest jobs first.
func (r *ImportJobRepository) Recent(ctx context.Context, limit int64) ([]domain.ImportJob, error) {
	cur, err := r.coll.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	out := []domain.ImportJob{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SetRow writes how the import ended for the row at index.
func (r *ImportJobRepository) SetRow(ctx context.Context, id domain.ID, index int, status, reason string) error {
	key := "rows." + strconv.Itoa(index)
	set := bson.M{key + ".status": status, key + ".reason": reason}
	update := bson.M{"$set": set}
	if reason == "" {
		delete(set, key+".reason")
		update["$unset"] = bson.M{key + ".reason": ""}
	}
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, update)
	return err
}

// SetProgress records the TikTok id being checked and, while TikTok limits the calls, when the next try is.
func (r *ImportJobRepository) SetProgress(ctx context.Context, id domain.ID, current string, waitUntil *time.Time, attempt int) error {
	set, unset := bson.M{}, bson.M{}
	if current != "" {
		set["current"] = current
	} else {
		unset["current"] = ""
	}
	if waitUntil != nil {
		set["waitUntil"], set["waitAttempt"] = *waitUntil, attempt
	} else {
		unset["waitUntil"], unset["waitAttempt"] = "", ""
	}
	update := bson.M{} // MongoDB refuses an empty $set or $unset, so only the non-empty ones are sent
	if len(set) > 0 {
		update["$set"] = set
	}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, update)
	return err
}

// RequestStop asks a running job to stop after the row it is working on. It reports whether a running job was found.
func (r *ImportJobRepository) RequestStop(ctx context.Context, id string) (bool, error) {
	oid, err := domain.ParseID(id)
	if err != nil {
		return false, domain.NotFound("Không tìm thấy đợt nhập.")
	}
	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": oid, "status": domain.ImportRunning}, bson.M{"$set": bson.M{"stopRequested": true}})
	if err != nil {
		return false, err
	}
	return res.MatchedCount > 0, nil
}

// StopRequested reports whether somebody asked the job to stop.
func (r *ImportJobRepository) StopRequested(ctx context.Context, id domain.ID) (bool, error) {
	var doc struct {
		StopRequested bool `bson:"stopRequested"`
	}
	err := r.coll.FindOne(ctx, bson.M{"_id": id}, options.FindOne().SetProjection(bson.M{"stopRequested": 1})).Decode(&doc)
	if err != nil {
		return false, err
	}
	return doc.StopRequested, nil
}

// Finish ends the job. The rows still queued stay queued: they are what the person runs again.
func (r *ImportJobRepository) Finish(ctx context.Context, id domain.ID, status, reason string, at time.Time) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set":   bson.M{"status": status, "stopReason": reason, "finishedAt": at},
		"$unset": bson.M{"current": "", "waitUntil": "", "waitAttempt": ""},
	})
	return err
}
