package utils

import (
	"math/rand"
	"strconv"
	"time"
)

// RandomSuffix provides a random string to append to pods,services,rcs.
func RandomSuffix() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return strconv.Itoa(r.Int() % 10000)
}
