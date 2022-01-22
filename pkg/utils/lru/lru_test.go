package lru

import (
	"fmt"
	"testing"
)

func TestLRU(t *testing.T) {
	lru := NewLRU(&Config{
		MaxSize: 20,
		RemoveFn: func(key string, value interface{}) {
			t.Logf("RemoveFn(%s, %d)", key, value)
		},
		SizeFn: func(key string, value interface{}) int64 {
			t.Logf("SizeFn(%s, %d)", key, value)
			return value.(int64)
		},
		AddFn: func(key string, value interface{}) {
			t.Logf("AddFn(%s, %d)", key, value)
		},
	})
	var i int64 = 1
	for j := 0; j < 10; j++ {
		lru.Add(fmt.Sprintf("%d", i), i)
		i++
	}
}

func BenchmarkLRU(t *testing.B) {
	lru := NewLRU(&Config{
		MaxSize: 20,
		RemoveFn: func(key string, value interface{}) {
		},
		SizeFn: func(key string, value interface{}) int64 {
			return int64(len(value.([]byte)))
		},
		AddFn: func(key string, value interface{}) {
		},
	})
	for i := 0; i < t.N; i++ {
		lru.Add(fmt.Sprintf("%d", i), make([]byte, 800))
	}
}
