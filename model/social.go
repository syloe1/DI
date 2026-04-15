package model

import "gorm.io/gorm"

type UserRelation struct {
	gorm.Model
	FromUID uint   `gorm:"not null;index:idx_relation_unique" json:"from_uid"`
	ToUID   uint   `gorm:"not null;index:idx_relation_unique" json:"to_uid"`
	Type    string `gorm:"size:16;not null;index:idx_relation_unique" json:"type"`
}
