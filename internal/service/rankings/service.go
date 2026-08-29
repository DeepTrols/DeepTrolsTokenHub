// Package rankings ports new-api's rankings analytics (model/vendor
// leaderboards, movers/droppers, token history and market-share series) on top
// of usage_logs. The aggregation is pure and unit-testable; DB access lives in
// the console handler.
package rankings

import (
	"sort"
	"time"
)

const (
	LeaderboardLimit = 20
	HistoryLimit     = 10
	VendorLimit      = 5
	MoverLimit       = 6
	OthersLabel      = "其他"
	UnknownVendor    = "未知"
)

// ModelTotal is one model's token volume over a period.
type ModelTotal struct {
	Model       string
	Provider    string
	TotalTokens int64
}

// BucketPoint is one model's token volume inside one time bucket.
type BucketPoint struct {
	Bucket   time.Time
	Model    string
	Provider string
	Tokens   int64
}

// Config describes the ranking window and bucket granularity.
type Config struct {
	ID          string
	Start, End  time.Time
	PrevStart   time.Time
	PrevEnd     time.Time
	BucketTrunc string // "hour" | "day" | "week"
	LabelLayout string
}

type RankedModel struct {
	Rank         int     `json:"rank"`
	PreviousRank *int    `json:"previous_rank,omitempty"`
	ModelName    string  `json:"model_name"`
	Vendor       string  `json:"vendor"`
	Category     string  `json:"category"`
	TotalTokens  int64   `json:"total_tokens"`
	Share        float64 `json:"share"`
	GrowthPct    float64 `json:"growth_pct"`
}

type RankedVendor struct {
	Rank        int     `json:"rank"`
	Vendor      string  `json:"vendor"`
	TotalTokens int64   `json:"total_tokens"`
	Share       float64 `json:"share"`
	GrowthPct   float64 `json:"growth_pct"`
	ModelsCount int     `json:"models_count"`
	TopModel    string  `json:"top_model"`
}

type Mover struct {
	ModelName   string  `json:"model_name"`
	Vendor      string  `json:"vendor"`
	RankDelta   int     `json:"rank_delta"`
	CurrentRank int     `json:"current_rank"`
	GrowthPct   float64 `json:"growth_pct"`
}

type HistoryPoint struct {
	Ts     string `json:"ts"`
	Label  string `json:"label"`
	Model  string `json:"model"`
	Vendor string `json:"vendor"`
	Tokens int64  `json:"tokens"`
}

type HistoryModel struct {
	Name   string `json:"name"`
	Vendor string `json:"vendor"`
	Total  int64  `json:"total"`
}

type HistorySeries struct {
	Points  []HistoryPoint `json:"points"`
	Models  []HistoryModel `json:"models"`
	Buckets int            `json:"buckets"`
}

type VendorSharePoint struct {
	Ts     string  `json:"ts"`
	Label  string  `json:"label"`
	Vendor string  `json:"vendor"`
	Share  float64 `json:"share"`
	Tokens int64   `json:"tokens"`
}

type VendorShareVendor struct {
	Name  string  `json:"name"`
	Total int64   `json:"total"`
	Share float64 `json:"share"`
}

type VendorShareSeries struct {
	Points  []VendorSharePoint  `json:"points"`
	Vendors []VendorShareVendor `json:"vendors"`
	Buckets int                 `json:"buckets"`
}

type Snapshot struct {
	Models             []RankedModel     `json:"models"`
	Vendors            []RankedVendor    `json:"vendors"`
	TopMovers          []Mover           `json:"top_movers"`
	TopDroppers        []Mover           `json:"top_droppers"`
	ModelsHistory      HistorySeries     `json:"models_history"`
	VendorShareHistory VendorShareSeries `json:"vendor_share_history"`
}

// BuildSnapshot assembles the full rankings snapshot from period totals,
// previous-period totals and per-bucket usage. Port of new-api's
// buildRankingsSnapshot.
func BuildSnapshot(cfg Config, totals, previous []ModelTotal, buckets []BucketPoint) *Snapshot {
	totalTokens := sumTokens(totals)
	prevRanks := rankMap(previous)
	prevTokens := tokenMap(previous)

	models := buildRankedModels(totals, totalTokens, prevRanks, prevTokens)
	vendors := buildRankedVendors(totals, previous, totalTokens)
	history := buildModelHistory(buckets, totals, cfg)
	vendorHistory := buildVendorShareHistory(buckets, vendors, totalTokens, cfg)
	movers, droppers := buildMovers(models)
	if movers == nil {
		movers = []Mover{}
	}
	if droppers == nil {
		droppers = []Mover{}
	}

	return &Snapshot{
		Models:             limitModels(models, LeaderboardLimit),
		Vendors:            vendors,
		TopMovers:          limitMovers(movers, MoverLimit),
		TopDroppers:        limitMovers(droppers, MoverLimit),
		ModelsHistory:      history,
		VendorShareHistory: vendorHistory,
	}
}

func sumTokens(totals []ModelTotal) int64 {
	var sum int64
	for _, t := range totals {
		sum += t.TotalTokens
	}
	return sum
}

func tokenMap(totals []ModelTotal) map[string]int64 {
	out := make(map[string]int64, len(totals))
	for _, t := range totals {
		out[t.Model] = t.TotalTokens
	}
	return out
}

func rankMap(totals []ModelTotal) map[string]int {
	rows := append([]ModelTotal(nil), totals...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].TotalTokens > rows[j].TotalTokens })
	out := make(map[string]int, len(rows))
	for i, r := range rows {
		out[r.Model] = i + 1
	}
	return out
}

func vendorName(provider string) string {
	if provider == "" {
		return UnknownVendor
	}
	return provider
}

func vendorOf(t ModelTotal) string {
	return vendorName(t.Provider)
}

func buildRankedModels(totals []ModelTotal, totalTokens int64, prevRanks map[string]int, prevTokens map[string]int64) []RankedModel {
	rows := append([]ModelTotal(nil), totals...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TotalTokens == rows[j].TotalTokens {
			return rows[i].Model < rows[j].Model
		}
		return rows[i].TotalTokens > rows[j].TotalTokens
	})
	out := make([]RankedModel, 0, len(rows))
	for i, item := range rows {
		var prevRank *int
		if r, ok := prevRanks[item.Model]; ok {
			rc := r
			prevRank = &rc
		}
		out = append(out, RankedModel{
			Rank:         i + 1,
			PreviousRank: prevRank,
			ModelName:    item.Model,
			Vendor:       vendorOf(item),
			Category:     "all",
			TotalTokens:  item.TotalTokens,
			Share:        share(item.TotalTokens, totalTokens),
			GrowthPct:    growthPct(item.TotalTokens, prevTokens[item.Model]),
		})
	}
	return out
}

func buildRankedVendors(totals, previous []ModelTotal, totalTokens int64) []RankedVendor {
	agg := map[string]*vendorAggregate{}
	for _, item := range totals {
		name := vendorOf(item)
		a := agg[name]
		if a == nil {
			a = &vendorAggregate{name: name, models: map[string]struct{}{}}
			agg[name] = a
		}
		a.totalTokens += item.TotalTokens
		a.models[item.Model] = struct{}{}
		if item.TotalTokens > a.topModelTokens {
			a.topModel = item.Model
			a.topModelTokens = item.TotalTokens
		}
	}
	for _, item := range previous {
		a := agg[vendorOf(item)]
		if a == nil {
			continue
		}
		a.previousTokens += item.TotalTokens
	}

	rows := make([]RankedVendor, 0, len(agg))
	for _, a := range agg {
		if a.totalTokens <= 0 {
			continue
		}
		rows = append(rows, RankedVendor{
			Vendor:      a.name,
			TotalTokens: a.totalTokens,
			Share:       share(a.totalTokens, totalTokens),
			GrowthPct:   growthPct(a.totalTokens, a.previousTokens),
			ModelsCount: len(a.models),
			TopModel:    a.topModel,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TotalTokens == rows[j].TotalTokens {
			return rows[i].Vendor < rows[j].Vendor
		}
		return rows[i].TotalTokens > rows[j].TotalTokens
	})
	for i := range rows {
		rows[i].Rank = i + 1
	}
	return rows
}

type vendorAggregate struct {
	name           string
	totalTokens    int64
	previousTokens int64
	models         map[string]struct{}
	topModel       string
	topModelTokens int64
}

func buildModelHistory(buckets []BucketPoint, totals []ModelTotal, cfg Config) HistorySeries {
	top := make(map[string]struct{})
	models := make([]HistoryModel, 0, HistoryLimit+1)
	var otherTotal int64
	for i, item := range totals {
		if i < HistoryLimit {
			top[item.Model] = struct{}{}
			models = append(models, HistoryModel{Name: item.Model, Vendor: vendorOf(item), Total: item.TotalTokens})
			continue
		}
		otherTotal += item.TotalTokens
	}
	if otherTotal > 0 {
		models = append(models, HistoryModel{Name: OthersLabel, Vendor: "Various", Total: otherTotal})
	}

	bucketSet := map[int64]struct{}{}
	tokens := map[int64]map[string]int64{}
	for _, b := range buckets {
		name := b.Model
		if _, ok := top[name]; !ok {
			name = OthersLabel
		}
		bucketSet[b.Bucket.Unix()] = struct{}{}
		if tokens[b.Bucket.Unix()] == nil {
			tokens[b.Bucket.Unix()] = map[string]int64{}
		}
		tokens[b.Bucket.Unix()][name] += b.Tokens
	}

	bucketsSorted := sortedBuckets(bucketSet)
	points := make([]HistoryPoint, 0, len(bucketsSorted)*len(models))
	for _, bucket := range bucketsSorted {
		ts := time.Unix(bucket, 0)
		for _, m := range models {
			t := tokens[bucket][m.Name]
			if t <= 0 {
				continue
			}
			points = append(points, HistoryPoint{
				Ts:     ts.Format(time.RFC3339),
				Label:  ts.Format(cfg.LabelLayout),
				Model:  m.Name,
				Vendor: m.Vendor,
				Tokens: t,
			})
		}
	}
	return HistorySeries{Points: points, Models: models, Buckets: len(bucketsSorted)}
}

func buildVendorShareHistory(buckets []BucketPoint, vendors []RankedVendor, totalTokens int64, cfg Config) VendorShareSeries {
	top := map[string]struct{}{}
	rows := make([]VendorShareVendor, 0, VendorLimit+1)
	var otherTotal int64
	for i, v := range vendors {
		if i < VendorLimit {
			top[v.Vendor] = struct{}{}
			rows = append(rows, VendorShareVendor{Name: v.Vendor, Total: v.TotalTokens, Share: v.Share})
			continue
		}
		otherTotal += v.TotalTokens
	}
	if otherTotal > 0 {
		rows = append(rows, VendorShareVendor{Name: OthersLabel, Total: otherTotal, Share: share(otherTotal, totalTokens)})
	}

	bucketSet := map[int64]struct{}{}
	tokens := map[int64]map[string]int64{}
	bucketTotals := map[int64]int64{}
	for _, b := range buckets {
		name := vendorName(b.Provider)
		if _, ok := top[name]; !ok {
			name = OthersLabel
		}
		bucketSet[b.Bucket.Unix()] = struct{}{}
		if tokens[b.Bucket.Unix()] == nil {
			tokens[b.Bucket.Unix()] = map[string]int64{}
		}
		tokens[b.Bucket.Unix()][name] += b.Tokens
		bucketTotals[b.Bucket.Unix()] += b.Tokens
	}

	bucketsSorted := sortedBuckets(bucketSet)
	points := make([]VendorSharePoint, 0, len(bucketsSorted)*len(rows))
	for _, bucket := range bucketsSorted {
		ts := time.Unix(bucket, 0)
		for _, v := range rows {
			t := tokens[bucket][v.Name]
			if t <= 0 {
				continue
			}
			points = append(points, VendorSharePoint{
				Ts:     ts.Format(time.RFC3339),
				Label:  ts.Format(cfg.LabelLayout),
				Vendor: v.Name,
				Share:  share(t, bucketTotals[bucket]),
				Tokens: t,
			})
		}
	}
	return VendorShareSeries{Points: points, Vendors: rows, Buckets: len(bucketsSorted)}
}

func buildMovers(models []RankedModel) (movers, droppers []Mover) {
	for _, m := range models {
		if m.PreviousRank == nil {
			continue
		}
		delta := *m.PreviousRank - m.Rank
		if delta == 0 {
			continue
		}
		row := Mover{ModelName: m.ModelName, Vendor: m.Vendor, RankDelta: delta, CurrentRank: m.Rank, GrowthPct: m.GrowthPct}
		if delta > 0 {
			movers = append(movers, row)
		} else {
			droppers = append(droppers, row)
		}
	}
	sort.Slice(movers, func(i, j int) bool {
		if movers[i].RankDelta == movers[j].RankDelta {
			return movers[i].GrowthPct > movers[j].GrowthPct
		}
		return movers[i].RankDelta > movers[j].RankDelta
	})
	sort.Slice(droppers, func(i, j int) bool {
		if droppers[i].RankDelta == droppers[j].RankDelta {
			return droppers[i].GrowthPct < droppers[j].GrowthPct
		}
		return droppers[i].RankDelta < droppers[j].RankDelta
	})
	return movers, droppers
}

func sortedBuckets(set map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(set))
	for b := range set {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func share(part, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(part) / float64(total)
}

func growthPct(current, previous int64) float64 {
	if previous <= 0 {
		if current > 0 {
			return 100
		}
		return 0
	}
	return (float64(current) - float64(previous)) / float64(previous) * 100
}

func limitModels(models []RankedModel, n int) []RankedModel {
	if len(models) > n {
		return models[:n]
	}
	return models
}

func limitMovers(movers []Mover, n int) []Mover {
	if len(movers) > n {
		return movers[:n]
	}
	return movers
}
