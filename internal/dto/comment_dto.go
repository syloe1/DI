package dto

type CreateCommentRequest struct {
	PostID   uint   `json:"post_id" binding:"required"`
	Content  string `json:"content" binding:"required"`
	ParentID uint   `json:"parent_id"`
}

type UpdateCommentRequest struct {
	Content string `json:"content" binding:"required"`
}
