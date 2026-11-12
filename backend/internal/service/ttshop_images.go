package service

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The product search used by the target-collaboration picker returns no pictures. A picture is on the product detail
// (one call per product), and TikTok shares one rate limit between every app of the group, so the pictures are read a
// few at a time, only for the products the page is showing, and kept for a day (a product's main picture rarely
// changes; a changed one shows up the next day).
const (
	productImageTTL      = 24 * time.Hour
	productImagesPerCall = 24
	productImageWorkers  = 4
)

// parseProductImage reads the first main picture (an https address) from a product detail, or "" when there is none.
func parseProductImage(raw []byte) string {
	var p struct {
		MainImages []struct {
			URLs      []string `json:"urls"`
			ThumbURLs []string `json:"thumb_urls"`
		} `json:"main_images"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return ""
	}
	for _, m := range p.MainImages {
		for _, list := range [][]string{m.URLs, m.ThumbURLs} {
			for _, u := range list {
				if strings.HasPrefix(u, "https://") {
					return u
				}
			}
		}
	}
	return ""
}

// ProductImages returns the main picture of each product id (up to productImagesPerCall of them). A product whose
// picture cannot be read is simply left out: the page shows a placeholder, and the next call tries it again.
func (s *TTShopService) ProductImages(ctx context.Context, ids []string) (map[string]string, error) {
	cipher, err := s.shopCipher(ctx)
	if err != nil {
		return nil, err
	}
	return s.productImages(ctx, ids, func(ctx context.Context, id string) (string, error) {
		raw, err := s.get(ctx, ttsProductGetPath+id, url.Values{"shop_cipher": {cipher}})
		if err != nil {
			return "", err
		}
		return parseProductImage(raw), nil
	}), nil
}

// productImages is ProductImages with the read of one picture passed in.
func (s *TTShopService) productImages(ctx context.Context, ids []string, fetch func(context.Context, string) (string, error)) map[string]string {
	out := map[string]string{}
	var todo []string
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if _, err := strconv.ParseUint(id, 10, 64); err != nil || seen[id] {
			continue
		}
		if len(seen) >= productImagesPerCall {
			break
		}
		seen[id] = true
		if img, age, ok := s.images.lookup(ctx, id); ok && age < productImageTTL && img != "" {
			out[id] = img
			continue
		}
		todo = append(todo, id)
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, productImageWorkers)
	for _, id := range todo {
		wg.Add(1)
		sem <- struct{}{}
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()
			img, err := fetch(ctx, id)
			if err != nil || img == "" {
				return
			}
			s.images.put(ctx, id, img)
			mu.Lock()
			out[id] = img
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	return out
}
