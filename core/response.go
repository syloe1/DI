package core

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

func Success(c *gin.Context, data interface{}) {
	Respond(c, http.StatusOK, "success", data)
}

func SuccessWithMessage(c *gin.Context, msg string, data interface{}) {
	Respond(c, http.StatusOK, msg, data)
}

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
