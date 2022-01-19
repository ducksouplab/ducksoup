package helpers

import (
	"os"
)

// function "synonym" whose interest is only to be sure init has been called first
func Getenv(key string) string {
	return os.Getenv(key)
}

func GetenvOr(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}
