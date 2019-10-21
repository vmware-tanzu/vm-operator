package common

import (
	"math/rand"
	"os"
	"strconv"
	"time"
)

const (
	defaultConcurrentReconciles = 1
	charset                     = "0123456789abcdef"
)

func GetMaxReconcileNum() int {
	// Get worker number for reconcile concurrently
	i, err := strconv.Atoi(os.Getenv("MAX_CONCURRENT_RECONCILES"))
	if err != nil {
		i = defaultConcurrentReconciles
	}
	return i
}

func RandomString(length int) string {
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
