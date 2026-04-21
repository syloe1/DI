package core

import (
	"regexp"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func RegisterCustomValidators() error {
	engine, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return nil
	}

	return engine.RegisterValidation("username_format", func(fl validator.FieldLevel) bool {
		return usernamePattern.MatchString(fl.Field().String())
	})
}
