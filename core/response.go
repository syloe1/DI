package core

import (
	"github.com/gin-gonic/gin"
)

type Response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

type ErrorResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(200, Response{
		Code: 200,
		Msg:  "success",
		Data: data,
	})
}
func Fail(c *gin.Context, code int, msg string) {
	c.JSON(code, ErrorResponse{
		Code: code,
		Msg:  msg,
	})
}
