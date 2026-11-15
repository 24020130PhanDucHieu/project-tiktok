package repository

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/jpeg" // registers the decoders image.DecodeConfig needs
	_ "image/png"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tiktoday/kocflow-api/internal/campaignseed"
	"github.com/tiktoday/kocflow-api/internal/domain"
)

// CampaignRepository keeps the campaigns ("campaigns") and their pictures ("campaign_images").
type CampaignRepository struct {
	campaigns *mongo.Collection
	images    *mongo.Collection
}

// NewCampaignRepository builds the repository.
func NewCampaignRepository(db *mongo.Database) *CampaignRepository {
	return &CampaignRepository{campaigns: db.Collection("campaigns"), images: db.Collection("campaign_images")}
}

// List returns the campaigns by name.
func (r *CampaignRepository) List(ctx context.Context) ([]domain.Campaign, error) {
	cur, err := r.campaigns.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "createdOrder", Value: 1}, {Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	out := []domain.Campaign{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Find returns one campaign, or a NotFound fault.
func (r *CampaignRepository) Find(ctx context.Context, id string) (*domain.Campaign, error) {
	oid, err := domain.ParseID(id)
	if err != nil {
		return nil, domain.NotFound("Không tìm thấy chiến dịch.")
	}
	var c domain.Campaign
	if err := r.campaigns.FindOne(ctx, bson.M{"_id": oid}).Decode(&c); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.NotFound("Không tìm thấy chiến dịch.")
		}
		return nil, err
	}
	return &c, nil
}

// SaveSteps replaces the steps of a campaign.
func (r *CampaignRepository) SaveSteps(ctx context.Context, id string, steps []domain.CampaignStep, at time.Time) error {
	oid, err := domain.ParseID(id)
	if err != nil {
		return domain.NotFound("Không tìm thấy chiến dịch.")
	}
	res, err := r.campaigns.UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"steps": steps, "updatedAt": at}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return domain.NotFound("Không tìm thấy chiến dịch.")
	}
	return nil
}

// AddImage stores a picture and sets its ID and CreatedAt.
func (r *CampaignRepository) AddImage(ctx context.Context, img *domain.CampaignImage) error {
	img.ID, img.CreatedAt = domain.NewID(), time.Now()
	_, err := r.images.InsertOne(ctx, img)
	return err
}

// Image returns one picture with its bytes, or a NotFound fault.
func (r *CampaignRepository) Image(ctx context.Context, id string) (*domain.CampaignImage, error) {
	oid, err := domain.ParseID(id)
	if err != nil {
		return nil, domain.NotFound("Không tìm thấy ảnh.")
	}
	var img domain.CampaignImage
	if err := r.images.FindOne(ctx, bson.M{"_id": oid}).Decode(&img); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.NotFound("Không tìm thấy ảnh.")
		}
		return nil, err
	}
	return &img, nil
}

// ImagesExist reports whether every id is a stored picture.
func (r *CampaignRepository) ImagesExist(ctx context.Context, ids []domain.ID) (bool, error) {
	if len(ids) == 0 {
		return true, nil
	}
	uniq := map[domain.ID]bool{}
	for _, id := range ids {
		uniq[id] = true
	}
	list := make([]domain.ID, 0, len(uniq))
	for id := range uniq {
		list = append(list, id)
	}
	n, err := r.images.CountDocuments(ctx, bson.M{"_id": bson.M{"$in": list}})
	return int(n) == len(list), err
}

// EnsureSeed stores the seeded campaigns (and their pictures) when there is no campaign yet.
func (r *CampaignRepository) EnsureSeed(ctx context.Context, seed []campaignseed.Campaign) error {
	if n, err := r.campaigns.CountDocuments(ctx, bson.M{}); err != nil || n > 0 {
		return err
	}
	imageIDs := map[string]domain.ID{} // the same file is stored once
	for i, c := range seed {
		steps := make([]domain.CampaignStep, len(c.Steps))
		for j, s := range c.Steps {
			steps[j] = domain.CampaignStep{Kind: s.Kind, Text: s.Text, Caption: s.Caption}
			if s.Kind != domain.StepImage {
				continue
			}
			id, ok := imageIDs[s.File]
			if !ok {
				data, err := campaignseed.Image(s.File)
				if err != nil {
					return err
				}
				cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
				if err != nil {
					return err
				}
				img := &domain.CampaignImage{Filename: s.File, ContentType: "image/" + format, Width: cfg.Width, Height: cfg.Height, Size: len(data), Data: data}
				if err := r.AddImage(ctx, img); err != nil {
					return err
				}
				id = img.ID
				imageIDs[s.File] = id
			}
			steps[j].ImageID = id
		}
		doc := bson.M{"_id": domain.NewID(), "name": c.Name, "steps": steps, "updatedAt": time.Now(), "createdOrder": i}
		if _, err := r.campaigns.InsertOne(ctx, doc); err != nil {
			return err
		}
	}
	return nil
}
