package core

import (
	"regexp"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// 编译正则表达式（全局只编译一次，性能更高）

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func RegisterCustomValidators() error {
	//断言成validator.Validate才能注册自定义规则
	engine, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return nil
	}

	return engine.RegisterValidation("username_format", func(fl validator.FieldLevel) bool {
		return usernamePattern.MatchString(fl.Field().String())
	})
}
