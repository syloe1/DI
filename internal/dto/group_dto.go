package dto

type CreateGroupRequest struct {
	Name      string `json:"name" binding:"required,max=100"`
	MemberIDs []uint `json:"member_ids" binding:"required,min=1"`
}
type SetGroupAdminRequest struct {
	UserID uint `json:"user_id" binding:"required,min=1"`
}
type TransferGroupOwnerRequest struct {
	UserID uint `json:"user_id" binding:"required,min=1"`
}
type InviteGroupMemberRequest struct {
	InviteeID uint `json:"invitee_id" binding:"required,min=1"`
}
type ReviewGroupInvitationRequest struct {
	Action string `json:"action" binding:"required,oneof=accept reject"`
}
type ApplyJoinGroupRequest struct {
	Reason string `json:"reason" binding:"omitempty,max=255"`
}
type ReviewJoinRequestRequest struct {
	Action string `json:"action" binding:"required,oneof=approve reject"`
}
type SendGroupMessageRequest struct {
	Content string `json:"content" binding:"required"`
}
type GroupMesssageListQuery struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"pageSize" binding:"omitempty,min=1"`
}
