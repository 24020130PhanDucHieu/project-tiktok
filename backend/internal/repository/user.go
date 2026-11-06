package repository

import (
	"context"
	"errors"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tiktoday/kocflow-api/internal/domain"
)

// UserRepository persists User documents.
type UserRepository struct{ coll *mongo.Collection }

// NewUserRepository builds the repository on the "users" collection.
func NewUserRepository(db *mongo.Database) *UserRepository {
	return &UserRepository{coll: db.Collection("users")}
}

func one(ctx context.Context, c *mongo.Collection, filter bson.M) (*domain.User, error) {
	var u domain.User
	err := c.FindOne(ctx, filter).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &u, err
}

// FindByEmail returns the user with the (already normalised) email, or (nil, nil).
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	return one(ctx, r.coll, bson.M{"email": email})
}

// FindByID returns a user by hex id, or (nil, nil).
func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	oid, err := domain.ParseID(id)
	if err != nil {
		return nil, nil
	}
	return one(ctx, r.coll, bson.M{"_id": oid})
}

// Create inserts a user.
func (r *UserRepository) Create(ctx context.Context, u *domain.User) error {
	now := time.Now()
	u.CreatedAt, u.UpdatedAt = now, now
	if u.Status == "" {
		u.Status = domain.UserActive
	}
	res, err := r.coll.InsertOne(ctx, u)
	if err != nil {
		return err
	}
	u.ID = domain.ID(res.InsertedID.(primitive.ObjectID))
	return nil
}

// patchUpdate is the update document of a partial change (updatedAt is added automatically).
func patchUpdate(ch domain.UserChange, now time.Time) bson.M {
	set := bson.M{"updatedAt": now}
	if ch.Name != nil {
		set["name"] = *ch.Name
	}
	if ch.Role != nil {
		set["role"] = *ch.Role
	}
	if ch.Status != nil {
		set["status"] = *ch.Status
	}
	update := bson.M{"$set": set}
	if ch.RevokeTokens {
		update["$inc"] = bson.M{"tokenVersion": 1}
	}
	return update
}

// Patch applies a partial change to an account.
func (r *UserRepository) Patch(ctx context.Context, id domain.ID, ch domain.UserChange) error {
	_, err := r.coll.UpdateByID(ctx, id, patchUpdate(ch, time.Now()))
	return err
}

// Rename changes the display name.
func (r *UserRepository) Rename(ctx context.Context, id domain.ID, name string) error {
	return r.Patch(ctx, id, domain.UserChange{Name: &name})
}

// SetPassword stores a new password hash, clears the failure counter and revokes every earlier session.
func (r *UserRepository) SetPassword(ctx context.Context, id domain.ID, hash string, at time.Time) error {
	_, err := r.coll.UpdateByID(ctx, id, bson.M{
		"$set": bson.M{"passwordHash": hash, "passwordChangedAt": at, "failedLogins": 0, "updatedAt": time.Now()},
		"$inc": bson.M{"tokenVersion": 1},
	})
	return err
}

// ClearLock lifts a temporary lock after too many failed sign-ins.
func (r *UserRepository) ClearLock(ctx context.Context, id domain.ID) error {
	_, err := r.coll.UpdateByID(ctx, id, bson.M{"$set": bson.M{"lockedUntil": nil, "updatedAt": time.Now()}})
	return err
}

// RecordLoginSuccess clears failure counters and stamps lastLoginAt.
func (r *UserRepository) RecordLoginSuccess(ctx context.Context, id domain.ID) error {
	now := time.Now()
	_, err := r.coll.UpdateByID(ctx, id, bson.M{
		"$set":   bson.M{"lastLoginAt": now, "failedLogins": 0, "updatedAt": now},
		"$unset": bson.M{"lockedUntil": ""},
	})
	return err
}

// RecordLoginFailure increments the failure counter and locks the account once max is reached.
func (r *UserRepository) RecordLoginFailure(ctx context.Context, u *domain.User, max int, lock time.Duration) error {
	update := bson.M{"$inc": bson.M{"failedLogins": 1}, "$set": bson.M{"updatedAt": time.Now()}}
	if u.FailedLogins+1 >= max {
		update = bson.M{"$set": bson.M{"failedLogins": 0, "lockedUntil": time.Now().Add(lock), "updatedAt": time.Now()}}
	}
	_, err := r.coll.UpdateByID(ctx, u.ID, update)
	return err
}

// userSearchCondition is the MongoDB condition of the admin user list.
func userSearchCondition(q domain.UserQuery) bson.M {
	filter := bson.M{}
	if q.Role != "" {
		filter["role"] = q.Role
	}
	if q.Status != "" {
		filter["status"] = q.Status
	}
	if q.Search != "" {
		re := primitive.Regex{Pattern: regexp.QuoteMeta(q.Search), Options: "i"}
		filter["$or"] = []bson.M{{"name": re}, {"email": re}}
	}
	return filter
}

// List returns a page of users (newest first) and the total.
func (r *UserRepository) List(ctx context.Context, q domain.UserQuery) ([]domain.User, int64, error) {
	filter := userSearchCondition(q)
	cur, err := r.coll.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetSkip(q.Skip).SetLimit(q.Limit))
	if err != nil {
		return nil, 0, err
	}
	items := []domain.User{}
	if err := cur.All(ctx, &items); err != nil {
		return nil, 0, err
	}
	total, err := r.coll.CountDocuments(ctx, filter)
	return items, total, err
}

func adminsCondition() bson.M { return bson.M{"role": domain.RoleAdmin} }

func activeAdminsCondition() bson.M {
	return bson.M{"role": domain.RoleAdmin, "status": bson.M{"$ne": domain.UserSuspended}}
}

// CountAdmins counts the administrators.
func (r *UserRepository) CountAdmins(ctx context.Context) (int64, error) {
	return r.coll.CountDocuments(ctx, adminsCondition())
}

// CountActiveAdmins counts the administrators that are not suspended.
func (r *UserRepository) CountActiveAdmins(ctx context.Context) (int64, error) {
	return r.coll.CountDocuments(ctx, activeAdminsCondition())
}

// Counts returns the account counters of the system page; "active" means signed in since the given moment.
func (r *UserRepository) Counts(ctx context.Context, activeSince time.Time) domain.UserCounts {
	count := func(filter bson.M) int64 {
		n, _ := r.coll.CountDocuments(ctx, filter)
		return n
	}
	return domain.UserCounts{
		Total:        count(bson.M{}),
		Admins:       count(adminsCondition()),
		Suspended:    count(bson.M{"status": domain.UserSuspended}),
		ActiveLast7d: count(bson.M{"lastLoginAt": bson.M{"$gte": activeSince}}),
	}
}

// Delete removes a user.
func (r *UserRepository) Delete(ctx context.Context, id domain.ID) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// DeleteAll removes every user (seeder only).
func (r *UserRepository) DeleteAll(ctx context.Context) error {
	_, err := r.coll.DeleteMany(ctx, bson.M{})
	return err
}

// ---- password resets ----------------------------------------------------------------------

// ResetRepository persists password-reset tokens.
type ResetRepository struct{ coll *mongo.Collection }

// NewResetRepository builds the repository on "password_resets".
func NewResetRepository(db *mongo.Database) *ResetRepository {
	return &ResetRepository{coll: db.Collection("password_resets")}
}

// Create stores a new token hash (older unused tokens of the user are invalidated first).
func (r *ResetRepository) Create(ctx context.Context, p *domain.PasswordReset) error {
	_, _ = r.coll.DeleteMany(ctx, bson.M{"userId": p.UserID})
	p.CreatedAt = time.Now()
	_, err := r.coll.InsertOne(ctx, p)
	return err
}

// Active returns the user's current unused, unexpired code, or (nil, nil).
func (r *ResetRepository) Active(ctx context.Context, userID domain.ID) (*domain.PasswordReset, error) {
	var p domain.PasswordReset
	err := r.coll.FindOne(ctx,
		bson.M{"userId": userID, "usedAt": bson.M{"$exists": false}, "expiresAt": bson.M{"$gt": time.Now()}},
		options.FindOne().SetSort(bson.D{{Key: "createdAt", Value: -1}}),
	).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &p, err
}

// Fail counts a wrong guess.
func (r *ResetRepository) Fail(ctx context.Context, id domain.ID) error {
	_, err := r.coll.UpdateByID(ctx, id, bson.M{"$inc": bson.M{"attempts": 1}})
	return err
}

// MarkUsed consumes the code atomically; false means it was already used.
func (r *ResetRepository) MarkUsed(ctx context.Context, id domain.ID) (bool, error) {
	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": id, "usedAt": bson.M{"$exists": false}}, bson.M{"$set": bson.M{"usedAt": time.Now()}})
	if err != nil {
		return false, err
	}
	return res.ModifiedCount == 1, nil
}

// ---- audit log -----------------------------------------------------------------------------

// AuditRepository persists audit log entries.
type AuditRepository struct{ coll *mongo.Collection }

// NewAuditRepository builds the repository on "audit_logs".
func NewAuditRepository(db *mongo.Database) *AuditRepository {
	return &AuditRepository{coll: db.Collection("audit_logs")}
}

// Insert writes one entry.
func (r *AuditRepository) Insert(ctx context.Context, a *domain.AuditLog) error {
	a.CreatedAt = time.Now()
	_, err := r.coll.InsertOne(ctx, a)
	return err
}

// List returns a page of entries (newest first) optionally filtered by action prefix or actor email.
func (r *AuditRepository) List(ctx context.Context, action, q string, skip, limit int64) ([]domain.AuditLog, int64, error) {
	filter := bson.M{}
	if action != "" {
		filter["action"] = primitive.Regex{Pattern: "^" + regexp.QuoteMeta(action)}
	}
	if q != "" {
		filter["actorEmail"] = primitive.Regex{Pattern: regexp.QuoteMeta(q), Options: "i"}
	}
	cur, err := r.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetSkip(skip).SetLimit(limit))
	if err != nil {
		return nil, 0, err
	}
	items := []domain.AuditLog{}
	if err := cur.All(ctx, &items); err != nil {
		return nil, 0, err
	}
	total, err := r.coll.CountDocuments(ctx, filter)
	return items, total, err
}
