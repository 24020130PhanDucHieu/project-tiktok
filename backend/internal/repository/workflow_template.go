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

// WorkflowTemplateRepository keeps the saved workflows ("workflow_templates").
type WorkflowTemplateRepository struct{ coll *mongo.Collection }

// NewWorkflowTemplateRepository builds the repository.
func NewWorkflowTemplateRepository(db *mongo.Database) *WorkflowTemplateRepository {
	return &WorkflowTemplateRepository{coll: db.Collection("workflow_templates")}
}

var errTemplateNotFound = domain.NotFound("Không tìm thấy quy trình đã lưu.")

// ErrTooManyTemplates is what Create answers when the shop already keeps the most it may.
var ErrTooManyTemplates = domain.Conflict("Đã lưu tối đa 50 quy trình. Hãy xóa bớt một quy trình cũ.")

// List returns the templates, the most recently changed first (at most the limit of the shop).
func (r *WorkflowTemplateRepository) List(ctx context.Context) ([]domain.WorkflowTemplate, error) {
	cur, err := r.coll.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}}).SetLimit(domain.MaxWorkflowTemplates))
	if err != nil {
		return nil, err
	}
	out := []domain.WorkflowTemplate{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Find returns one template, or a NotFound fault.
func (r *WorkflowTemplateRepository) Find(ctx context.Context, id string) (*domain.WorkflowTemplate, error) {
	oid, err := domain.ParseID(id)
	if err != nil {
		return nil, errTemplateNotFound
	}
	var t domain.WorkflowTemplate
	if err := r.coll.FindOne(ctx, bson.M{"_id": oid}).Decode(&t); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, errTemplateNotFound
		}
		return nil, err
	}
	return &t, nil
}

// Create stores a template and sets its ID and times. It refuses past the limit of the shop.
func (r *WorkflowTemplateRepository) Create(ctx context.Context, t *domain.WorkflowTemplate) error {
	n, err := r.coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		return err
	}
	if n >= domain.MaxWorkflowTemplates {
		return ErrTooManyTemplates
	}
	now := time.Now()
	t.ID, t.CreatedAt, t.UpdatedAt = domain.NewID(), now, now
	if t.Edges == nil {
		t.Edges = []domain.WorkflowEdge{}
	}
	_, err = r.coll.InsertOne(ctx, t)
	return err
}

// Update replaces the name and the drawing of a template.
func (r *WorkflowTemplateRepository) Update(ctx context.Context, id, name string, nodes []domain.WorkflowNode, edges []domain.WorkflowEdge, at time.Time) error {
	oid, err := domain.ParseID(id)
	if err != nil {
		return errTemplateNotFound
	}
	if edges == nil {
		edges = []domain.WorkflowEdge{}
	}
	res, err := r.coll.UpdateByID(ctx, oid, bson.M{"$set": bson.M{"name": name, "nodes": nodes, "edges": edges, "updatedAt": at}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return errTemplateNotFound
	}
	return nil
}

// Delete removes a template, or answers NotFound.
func (r *WorkflowTemplateRepository) Delete(ctx context.Context, id string) error {
	oid, err := domain.ParseID(id)
	if err != nil {
		return errTemplateNotFound
	}
	res, err := r.coll.DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return errTemplateNotFound
	}
	return nil
}
