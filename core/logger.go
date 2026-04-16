package core

import (
	"log"
	"os"
)

func NewLogger() *log.Logger {
	return log.New(os.Stdout, "[go-admin] ", log.LstdFlags|log.Lshortfile)
}
