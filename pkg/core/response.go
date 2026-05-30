package core

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

// 基础成功 Success
func Success(c *gin.Context, data interface{}) {
	Respond(c, http.StatusOK, "success", data)
}

// 自定义文案成功 SuccessWithMessage
func SuccessWithMessage(c *gin.Context, msg string, data interface{}) {
	Respond(c, http.StatusOK, msg, data)
}

// 只handler使用
// 手动指定状态码和错误文案，无返回数据，用于主动判断业务失败场景。
func Fail(c *gin.Context, code int, msg string) {
	Respond(c, code, msg, nil)
}

func Respond(c *gin.Context, code int, msg string, data interface{}) {
	if msg == "" {
		msg = http.StatusText(code)
	}

	c.JSON(code, Response{
		Code: code,
		Msg:  msg,
		Data: data,
	})
}

// 自定义业务错误 BizError
type BizError struct {
	Code int    `json:"code"`
	Msg  string `json:"message"`
}

/*
Go 规定：只要一个类型实现了 Error() string 方法，它就是 error 类型。
// 标准 error 接口长这样（语言内置）

	type error interface {
	    Error() string
	}
*/
func (c *BizError) Error() string {
	return c.Msg
}
func NewBizError(code int, msg string) *BizError {
	return &BizError{
		Code: code,
		Msg:  msg,
	}
}

// 自动根据 error 返回错误（最常用！）
func FailByError(c *gin.Context, err error) {
	var bizError *BizError
	if errors.As(err, &bizError) {
		Respond(c, bizError.Code, bizError.Msg, nil)
		return
	}
	Respond(c, http.StatusInternalServerError, err.Error(), nil)
}
