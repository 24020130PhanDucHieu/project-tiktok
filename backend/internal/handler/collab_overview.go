package handler

import (
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/service"
)

const (
	overviewLoadLimit = 5000
	overviewMaxItems  = 300
)

type overviewKoc struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Handle    string `json:"handle"`
	AvatarURL string `json:"avatarUrl"`
}

// overviewItem is one row of the management table: the collaboration plus who it is with.
type overviewItem struct {
	domain.Collab
	Koc overviewKoc `json:"koc"`
}

// matches reports whether a row passes the status / mode / text filters of the management tab.
func (it overviewItem) matches(status, mode, q string) bool {
	if status != "" && it.Status != status {
		return false
	}
	if mode != "" {
		have := it.Mode
		if have == "" {
			have = "none"
		}
		if have != mode {
			return false
		}
	}
	if q = strings.ToLower(strings.TrimSpace(q)); q != "" {
		hay := strings.ToLower(it.Koc.Name + " " + it.Koc.Handle + " " + it.ProductName + " " + it.Name)
		if !strings.Contains(hay, q) {
			return false
		}
	}
	return true
}

// Overview handles GET /collabs/overview: totals, a time series (day / week / month / year) and the table rows
// of every collaboration whose start day is inside the window.
func (h *CollabHandler) Overview(c *fiber.Ctx) error {
	gran := c.Query("granularity", service.GranDay)
	now := time.Now()
	from, to := service.DefaultOverviewRange(now, gran)
	var err error
	if v := c.Query("from"); v != "" {
		if from, err = service.ParseOverviewDay(v); err != nil {
			return httpx.NewError(fiber.StatusBadRequest, "Ngày bắt đầu không hợp lệ (định dạng YYYY-MM-DD).")
		}
	}
	if v := c.Query("to"); v != "" {
		if to, err = service.ParseOverviewDay(v); err != nil {
			return httpx.NewError(fiber.StatusBadRequest, "Ngày kết thúc không hợp lệ (định dạng YYYY-MM-DD).")
		}
	}

	ctx := c.UserContext()
	cols, err := h.collabs.All(ctx, overviewLoadLimit)
	if err != nil {
		return err
	}
	ids := make([]primitive.ObjectID, 0, len(cols))
	seen := map[primitive.ObjectID]bool{}
	for i := range cols {
		if !seen[cols[i].KocID] {
			seen[cols[i].KocID] = true
			ids = append(ids, cols[i].KocID)
		}
	}
	kocs := map[primitive.ObjectID]overviewKoc{}
	if len(ids) > 0 {
		found, err := h.kocs.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
		if err != nil {
			return err
		}
		for _, k := range found {
			kocs[k.ID] = overviewKoc{ID: k.ID.Hex(), Name: k.DisplayName, Handle: "@" + strings.TrimPrefix(k.TiktokID, "@"), AvatarURL: k.AvatarURL}
		}
	}

	status, mode, q := c.Query("status"), c.Query("mode"), c.Query("q")
	filtered := make([]overviewItem, 0, len(cols))
	plain := make([]domain.Collab, 0, len(cols))
	for _, col := range cols {
		it := overviewItem{Collab: col, Koc: kocs[col.KocID]}
		if it.matches(status, mode, q) {
			filtered = append(filtered, it)
			plain = append(plain, col)
		}
	}
	ov, err := service.BuildCollabOverview(plain, gran, from, to)
	if err != nil {
		return httpx.NewError(fiber.StatusBadRequest, err.Error())
	}

	// table rows: same window, newest first
	fromDay, _ := service.ParseOverviewDay(ov.From)
	toDay, _ := service.ParseOverviewDay(ov.To)
	end := toDay.AddDate(0, 0, 1)
	rows := make([]overviewItem, 0, overviewMaxItems)
	total := 0
	for _, it := range filtered {
		if it.StartedAt.Before(fromDay) || !it.StartedAt.Before(end) {
			continue
		}
		total++
		rows = append(rows, it)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].StartedAt.After(rows[j].StartedAt) })
	if len(rows) > overviewMaxItems {
		rows = rows[:overviewMaxItems]
	}
	return httpx.OK(c, fiber.Map{"overview": ov, "items": rows, "total": total})
}
