package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// TargetInviteRepository persists TargetInvite documents on "target_invites".
type TargetInviteRepository struct {
	coll     *mongo.Collection
	settings *mongo.Collection
}

// NewTargetInviteRepository builds the repository.
func NewTargetInviteRepository(db *mongo.Database) *TargetInviteRepository {
	return &TargetInviteRepository{coll: db.Collection("target_invites"), settings: db.Collection("app_settings")}
}

// Insert stores a new record. If the request key already exists (unique index) nothing is inserted: it returns the
// existing record together with domain.ErrDuplicateRequest, so a repeated request can never send a second invitation.
func (r *TargetInviteRepository) Insert(ctx context.Context, inv *domain.TargetInvite) (*domain.TargetInvite, error) {
	now := time.Now()
	inv.CreatedAt, inv.UpdatedAt = now, now
	if inv.Products == nil {
		inv.Products = []domain.TargetInviteProduct{}
	}
	if inv.Creators == nil {
		inv.Creators = []domain.TargetInviteCreator{}
	}
	res, err := r.coll.InsertOne(ctx, inv)
	if err == nil {
		inv.ID = domain.ID(res.InsertedID.(primitive.ObjectID))
		return inv, nil
	}
	if mongo.IsDuplicateKeyError(err) {
		var existing domain.TargetInvite
		if findErr := r.coll.FindOne(ctx, bson.M{"requestKey": inv.RequestKey}).Decode(&existing); findErr == nil {
			return &existing, domain.ErrDuplicateRequest
		}
		return nil, domain.ErrDuplicateRequest
	}
	return nil, err
}

const sendingSwitchID = "target_invites_sending"

// SendingEnabled reports whether sending invitations to real creators is switched on. It is OFF until an admin turns
// it on, and any read problem counts as off.
func (r *TargetInviteRepository) SendingEnabled(ctx context.Context) (bool, error) {
	var doc struct {
		Enabled bool `bson:"enabled"`
	}
	err := r.settings.FindOne(ctx, bson.M{"_id": sendingSwitchID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return doc.Enabled, nil
}

// SetSendingEnabled turns sending on or off and remembers who did it.
func (r *TargetInviteRepository) SetSendingEnabled(ctx context.Context, enabled bool, by string) error {
	_, err := r.settings.UpdateOne(ctx, bson.M{"_id": sendingSwitchID},
		bson.M{"$set": bson.M{"enabled": enabled, "updatedBy": by, "updatedAt": time.Now()}}, options.Update().SetUpsert(true))
	return err
}

// SetResult records what TikTok answered.
func (r *TargetInviteRepository) SetResult(ctx context.Context, id domain.ID, status, targetID, errText string) error {
	set := bson.M{"status": status, "updatedAt": time.Now(), "error": errText}
	if targetID != "" {
		set["targetId"] = targetID
	}
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set})
	return err
}

// MarkCancelled flags the record of a cancelled invitation (no-op when it was not sent from this app).
func (r *TargetInviteRepository) MarkCancelled(ctx context.Context, targetID string) error {
	_, err := r.coll.UpdateMany(ctx, bson.M{"targetId": targetID}, bson.M{"$set": bson.M{"status": domain.TargetInviteCancelled, "updatedAt": time.Now()}})
	return err
}

// ByTargetIDs returns the local records of the given TikTok invitation ids, keyed by id.
func (r *TargetInviteRepository) ByTargetIDs(ctx context.Context, ids []string) (map[string]domain.TargetInvite, error) {
	out := map[string]domain.TargetInvite{}
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := r.coll.Find(ctx, bson.M{"targetId": bson.M{"$in": ids}}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var all []domain.TargetInvite
	if err := cur.All(ctx, &all); err != nil {
		return nil, err
	}
	for _, inv := range all {
		out[inv.TargetID] = inv
	}
	return out, nil
}

// ByTargetID returns the local record of one invitation, or (nil, nil).
func (r *TargetInviteRepository) ByTargetID(ctx context.Context, id string) (*domain.TargetInvite, error) {
	var inv domain.TargetInvite
	err := r.coll.FindOne(ctx, bson.M{"targetId": id}).Decode(&inv)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

const shopContactID = "target_invites_contact"

// ShopContact returns the contact remembered from the last invitation (empty when none).
func (r *TargetInviteRepository) ShopContact(ctx context.Context) (domain.ShopContact, error) {
	var doc domain.ShopContact
	err := r.settings.FindOne(ctx, bson.M{"_id": shopContactID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.ShopContact{}, nil
	}
	return doc, err
}

// SetShopContact remembers the contact used by the last invitation.
func (r *TargetInviteRepository) SetShopContact(ctx context.Context, c domain.ShopContact, by string) error {
	_, err := r.settings.UpdateOne(ctx, bson.M{"_id": shopContactID},
		bson.M{"$set": bson.M{"email": c.Email, "zalo": c.Zalo, "facebook": c.Facebook, "updatedBy": by, "updatedAt": time.Now()}},
		options.Update().SetUpsert(true))
	return err
}
