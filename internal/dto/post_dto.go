package dto

type CreatePostRequest struct {
	Title     string  `json:"title" binding:"required,min=1,max=128"`
	Content   string  `json:"content" binding:"required,min=1,max=5000"`
	IsPublic  bool    `json:"is_public"`
	Status    string  `json:"status" binding:"omitempty,oneof=published draft scheduled"`
	PublishAt *string `json:"publish_at" binding:"omitempty,datetime=2006-01-02T15:04:05Z07:00"`
	Topics    string  `json:"topics" binding:"omitempty,max=255"`
	Images    string  `json:"images" binding:"omitempty,max=512"`
}

type UpdatePostRequest struct {
	Title     string  `json:"title" binding:"omitempty,min=1,max=128"`
	Content   string  `json:"content" binding:"omitempty,min=1,max=5000"`
	IsPublic  *bool   `json:"is_public"`
	Status    string  `json:"status" binding:"omitempty,oneof=published draft scheduled"`
	PublishAt *string `json:"publish_at" binding:"omitempty,datetime=2006-01-02T15:04:05Z07:00"`
	Topics    string  `json:"topics" binding:"omitempty,max=255"`
	Images    string  `json:"images" binding:"omitempty,max=512"`
}

type PostListQuery struct {
	Topic    string `form:"topic" binding:"omitempty,max=64"`
	Sort     string `form:"sort" binding:"omitempty,oneof=time hot"`
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=50"`
}
