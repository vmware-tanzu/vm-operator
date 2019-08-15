package common

import (
	"os"
	"strconv"
)

const (
	defaultConcurrentReconciles = 1
)

func GetMaxReconcileNum() int {
	// Get worker number for reconcile concurrently
	i, err := strconv.Atoi(os.Getenv("MAX_CONCURRENT_RECONCILES"))
	if err != nil {
		i = defaultConcurrentReconciles
	}
	return i
}
