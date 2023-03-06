package helpers

import (
	"fmt"
	"math/rand"
	"time"
)

// from https://stackoverflow.com/a/65607935
func RandomHexString(length int) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, length)
	r.Read(b)
	return fmt.Sprintf("%x", b)[:length]
}
