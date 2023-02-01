package helpers

import (
	"fmt"
	"math/rand"
	"time"
)

// from https://stackoverflow.com/a/65607935
func RandomHexString(length int) string {
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, length)
	rand.Read(b)
	return fmt.Sprintf("%x", b)[:length]
}
