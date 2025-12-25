package engine

import (
	"testing"
)

func TestRoundRobinStrategy(t *testing.T) {
	rr := &RoundRobinStrategy{}

	// 카운트가 3인 경우: 1, 2, 0, 1, 2... 순서 확인 (atomic.AddUint64는 1부터 시작하므로 %3 -> 1, 2, 0, 1...)
	// count가 3일 때 로직상 (1%3)=1, (2%3)=2, (3%3)=0, (4%3)=1 ...

	count := 3
	expected := []int{1, 2, 0, 1, 2}

	for i, want := range expected {
		got := rr.Next(count, "")
		if got != want {
			t.Errorf("RoundRobin.Next() step %d = %d; want %d", i, got, want)
		}
	}
}

func TestHashStrategy(t *testing.T) {
	h := &HashStrategy{}
	count := 5

	key1 := "192.168.0.1"
	key2 := "192.168.0.2"

	// 같은 키는 항상 같은 인덱스를 반환해야 함
	idx1_first := h.Next(count, key1)
	idx1_second := h.Next(count, key1)

	if idx1_first != idx1_second {
		t.Errorf("HashStrategy consistency failed: %d != %d", idx1_first, idx1_second)
	}

	// 다른 키는 (높은 확률로) 다른 인덱스 또는 일관된 결과를 반환
	idx2 := h.Next(count, key2)
	t.Logf("Key1 -> %d, Key2 -> %d", idx1_first, idx2)
}
