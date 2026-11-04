package service

import "testing"

func TestEffectiveRange(t *testing.T) {
	list := map[string]float64{"a": 702703, "b": 702703}
	deal := promoEntry{rank: 0, skus: map[string]float64{"a": 550000, "b": 550000}}
	flash := promoEntry{rank: 1, skus: map[string]float64{"a": 360000, "b": 360000}}

	lo, hi, ok := effectiveRange(list, []promoEntry{deal, flash})
	if !ok || lo != 550000 || hi != 550000 {
		t.Fatalf("deal must win over flash sale, got %v %v %v", lo, hi, ok)
	}
	lo, hi, ok = effectiveRange(list, []promoEntry{flash})
	if !ok || lo != 360000 || hi != 360000 {
		t.Fatalf("flash only, got %v %v %v", lo, hi, ok)
	}
	// only one SKU discounted: the range spans the discounted and the untouched SKU
	lo, hi, ok = effectiveRange(list, []promoEntry{{skus: map[string]float64{"a": 500000}}})
	if !ok || lo != 500000 || hi != 702703 {
		t.Fatalf("partial discount, got %v %v %v", lo, hi, ok)
	}
	// a product-wide price applies to every SKU
	lo, hi, ok = effectiveRange(list, []promoEntry{{hasProduct: true, productWide: 600000, skus: map[string]float64{}}})
	if !ok || lo != 600000 || hi != 600000 {
		t.Fatalf("product wide, got %v %v %v", lo, hi, ok)
	}
	// activity price equal to list price is not a discount
	if _, _, ok = effectiveRange(list, []promoEntry{{skus: map[string]float64{"a": 702703, "b": 702703}}}); ok {
		t.Fatal("same price must not count as discounted")
	}
	if _, _, ok = effectiveRange(nil, []promoEntry{deal}); ok {
		t.Fatal("no SKU prices must not produce a range")
	}
}
