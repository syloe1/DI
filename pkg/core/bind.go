package core

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
)

func ParseBindError(err error) error {
	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		return NewBizError(http.StatusBadRequest, translateValidationError(validationErrors[0]))
	}
	return NewBizError(http.StatusBadRequest, "请求参数不合法")
}

func translateValidationError(fe validator.FieldError) string {
	fieldName := strings.ToLower(fe.Field())
	switch fe.Tag() {
	case "required":
		return fieldName + "不能为空"
	case "min":
		return fieldName + "长度过短"
	case "max":
		return fieldName + "长度超出限制"
	case "oneof":
		return fieldName + "取值不合法"
	case "datetime":
		return fieldName + "时间格式不正确"
	case "username_format":
		return fieldName + "只能包含字母、数字和下划线"
	default:
		return fieldName + "校验失败"
	}
}
