package engine

import (
	"hash/crc32"
	"math/rand"
	"sync/atomic"
)

type BalancerStrategy interface {
	Next(count int, key string) int
}

type RoundRobinStrategy struct {
	counter uint64
}

func (rr *RoundRobinStrategy) Next(count int, key string) int {
	if count == 0 {
		return -1
	}

	idx := atomic.AddUint64(&rr.counter, 1) % uint64(count)
	return int(idx)
}

type HashStrategy struct {
}

func (h *HashStrategy) Next(count int, key string) int {
	if count == 0 {
		return -1
	}

	checksum := crc32.ChecksumIEEE([]byte(key))
	return int(checksum % uint32(count))
}

type RandomStrategy struct {
}

func (r *RandomStrategy) Next(count int, key string) int {
	if count == 0 {
		return -1
	}
	return rand.Intn(count)
}