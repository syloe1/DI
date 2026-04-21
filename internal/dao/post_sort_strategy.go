package dao

import "gorm.io/gorm"

type PostSortStrategy interface {
	Name() string
	Apply(query *gorm.DB) *gorm.DB
}

type TimeSortStrategy struct{}

func (TimeSortStrategy) Name() string {
	return "time"
}

func (TimeSortStrategy) Apply(query *gorm.DB) *gorm.DB {
	return query.Order("created_at desc")
}

type HotSortStrategy struct{}

func (HotSortStrategy) Name() string {
	return "hot"
}

func (HotSortStrategy) Apply(query *gorm.DB) *gorm.DB {
	return query.
		Joins("LEFT JOIN (SELECT post_id, COUNT(*) AS like_count FROM likes GROUP BY post_id) lc ON lc.post_id = posts.id").
		Joins("LEFT JOIN (SELECT post_id, COUNT(*) AS collect_count FROM collects GROUP BY post_id) cc ON cc.post_id = posts.id").
		Joins("LEFT JOIN (SELECT post_id, COUNT(*) AS share_count FROM shares GROUP BY post_id) sc ON sc.post_id = posts.id").
		Order("COALESCE(lc.like_count, 0) + COALESCE(cc.collect_count, 0) + COALESCE(sc.share_count, 0) DESC").
		Order("posts.created_at desc")
}

func ResolvePostSortStrategy(sort string) PostSortStrategy {
	switch sort {
	case "", "time":
		return TimeSortStrategy{}
	case "hot":
		return HotSortStrategy{}
	default:
		return TimeSortStrategy{}
	}
}
