// Command seed resets the database and inserts demo data.
package main

import (
	"context"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/database"
	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/repository"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg := config.Load()

	client, db, err := database.Connect(ctx, cfg.MongoURI, cfg.MongoDBName)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Disconnect(ctx)
	if err := database.EnsureIndexes(ctx, db); err != nil {
		log.Fatal(err)
	}

	users, kocs := repository.NewUserRepository(db), repository.NewKocRepository(db)
	convs, msgs := repository.NewConversationRepository(db), repository.NewMessageRepository(db)
	for _, step := range []func(context.Context) error{users.DeleteAll, kocs.DeleteAll, convs.DeleteAll, msgs.DeleteAll} {
		if err := step(ctx); err != nil {
			log.Fatal(err)
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("Admin12345!"), 12)
	if err != nil {
		log.Fatal(err)
	}
	if err := users.Create(ctx, &domain.User{Name: "Workspace Admin", Email: "admin@tiktoday.local", PasswordHash: string(hash), Role: "admin"}); err != nil {
		log.Fatal(err)
	}

	mk := func(mod func(*domain.Koc)) *domain.Koc {
		k := domain.NewKoc()
		mod(k)
		return k
	}
	err = kocs.InsertMany(ctx, []*domain.Koc{
		mk(func(k *domain.Koc) {
			k.TiktokID, k.DisplayName, k.Followers, k.Gmv = "minhaireview", "Minh Ái 🎀", 12700, 1000000
			k.Category, k.ContentPillar, k.TargetAudience = "Làm đẹp", "Review skincare & unbox mỹ phẩm", "Nữ 18-28 tuổi"
			k.Source, k.Phone, k.Campaign, k.ProductName = "TikToday", "0327637213", "Đợt T11 - Serum B5", "Serum phục hồi B5"
			k.CommissionRate, k.CastType, k.CastFee, k.Stage = 15, "freecast", 0, domain.StageQualified
		}),
		mk(func(k *domain.Koc) {
			k.TiktokID, k.DisplayName, k.Followers, k.Gmv = "giadinhtraicay1202", "Gia đình trái cây 🍋🍊", 3100, 5000000
			k.Category, k.ContentPillar, k.TargetAudience = "Đời sống", "Vlog nấu ăn gia đình", "Mẹ bỉm & nội trợ 24-35"
			k.Source, k.Phone, k.Campaign, k.ProductName = "TikTok Liên kết", "0866073103", "Đợt T11 - Máy ép chậm", "Máy ép chậm mini"
			k.CommissionRate, k.CastType, k.CastFee, k.Stage = 10, "cast", 500000, domain.StageAwaitingApproval
		}),
		mk(func(k *domain.Koc) {
			k.TiktokID, k.DisplayName, k.Followers = "changditap", "Cô bé lười đi tập 🧚", 126
			k.Source, k.Stage = "TikTok", domain.StageSourced
		}),
	})
	if err != nil {
		log.Fatal(err)
	}

	conv := &domain.Conversation{
		Channel: domain.ChannelInternal, ContactName: "Gia đình trái cây 🍋🍊", ContactHandle: "@giadinhtraicay1202",
		LastMessage: "Mình đã nhận được thông tin rồi ạ", UnreadCount: 2, Status: domain.ConversationOpen,
		LastMessageAt: time.Now(),
	}
	if err := insertConversation(ctx, convs, conv); err != nil {
		log.Fatal(err)
	}
	err = msgs.InsertMany(ctx, []*domain.Message{
		{ConversationID: conv.ID, Direction: domain.DirectionInbound, Text: "Chào bạn, mình đã xem brief rồi ạ", Status: domain.MessageSent},
		{ConversationID: conv.ID, Direction: domain.DirectionOutbound, Text: "Chào bạn, bên mình muốn trao đổi thêm về chiến dịch mới nhé.", Status: domain.MessageSent},
		{ConversationID: conv.ID, Direction: domain.DirectionInbound, Text: "Mình đã nhận được thông tin rồi ạ", Status: domain.MessageSent},
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Seed completed")
}

func insertConversation(ctx context.Context, r *repository.ConversationRepository, c *domain.Conversation) error {
	return r.Create(ctx, c)
}
