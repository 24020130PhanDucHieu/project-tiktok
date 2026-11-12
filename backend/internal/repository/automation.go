package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const automationSwitchID = "automation_sending"

// AutomationRepository keeps the master switch for the messages the system sends to creators on its own. It is OFF
// until an admin turns it on, and a missing record means off.
type AutomationRepository struct{ settings *mongo.Collection }

// NewAutomationRepository builds the repository on "app_settings".
func NewAutomationRepository(db *mongo.Database) *AutomationRepository {
	return &AutomationRepository{settings: db.Collection("app_settings")}
}

// AutomationState is the switch with who set it last.
type AutomationState struct {
	Enabled   bool      `bson:"enabled" json:"enabled"`
	UpdatedBy string    `bson:"updatedBy" json:"updatedBy"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

// State reads the switch.
func (r *AutomationRepository) State(ctx context.Context) (AutomationState, error) {
	var st AutomationState
	err := r.settings.FindOne(ctx, bson.M{"_id": automationSwitchID}).Decode(&st)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return AutomationState{}, nil
	}
	return st, err
}

// Enabled reports whether the system may message creators on its own.
func (r *AutomationRepository) Enabled(ctx context.Context) (bool, error) {
	st, err := r.State(ctx)
	return st.Enabled, err
}

// SetEnabled turns the switch on or off and remembers who did it.
func (r *AutomationRepository) SetEnabled(ctx context.Context, enabled bool, by string) error {
	_, err := r.settings.UpdateOne(ctx, bson.M{"_id": automationSwitchID},
		bson.M{"$set": bson.M{"enabled": enabled, "updatedBy": by, "updatedAt": time.Now()}}, options.Update().SetUpsert(true))
	return err
}
