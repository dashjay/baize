package lru

import (
	"container/list"
	"github.com/dashjay/bazel-remote-exec/pkg/interfaces"
)

type RemoveFn func(key string, value interface{})
type SizeFn func(key string, value interface{}) int64
type AddFn func(key string, value interface{})

type Config struct {
	MaxSize  int64
	RemoveFn RemoveFn
	SizeFn   SizeFn
	AddFn    AddFn
}

type Entry struct {
	Key   string
	Value interface{}
}
type LRU struct {
	items       list.List
	itemsMap    map[string]*list.Element
	sizeFn      SizeFn
	removeFn    RemoveFn
	addFn       AddFn
	maxSize     int64
	currentSize int64
}

func NewLRU(cfg *Config) interfaces.LRU {
	return &LRU{
		items:       list.List{},
		itemsMap:    make(map[string]*list.Element),
		sizeFn:      cfg.SizeFn,
		removeFn:    cfg.RemoveFn,
		maxSize:     cfg.MaxSize,
		addFn:       cfg.AddFn,
		currentSize: 0,
	}
}

func (l *LRU) Add(key string, value interface{}) bool {
	if ent, ok := l.lookupItem(key); ok {
		l.items.MoveToFront(ent)
		ent.Value.(*Entry).Value = value
		return true
	}

	// Add new item
	l.addItem(key, value, true)

	if l.addFn != nil {
		l.addFn(key, value)
	}
	for l.currentSize > l.maxSize {
		l.removeOldest()
	}
	return true
}

func (l *LRU) PushBack(key string, value interface{}) bool {
	if ele, ok := l.lookupItem(key); ok {
		ele.Value = value
		return true
	}
	// Add new item
	l.addItem(key, value, false)

	for l.currentSize > l.maxSize {
		l.removeOldest()
		return false
	}
	return true
}

func (l *LRU) Get(key string) (interface{}, bool) {
	if ele, ok := l.lookupItem(key); ok {
		l.items.MoveToFront(ele)
		if ele.Value == nil {
			return nil, false
		}
		return ele.Value, true
	}
	return nil, false
}

func (l *LRU) Contains(key string) bool {
	if ent, ok := l.lookupItem(key); ok {
		l.items.MoveToFront(ent)
		return true
	}
	return false
}

func (l *LRU) Remove(key string) bool {
	if val, ok := l.lookupItem(key); ok {
		l.removeElement(val)
		return true
	}
	return false
}

func (l *LRU) Purge() {
	for k := range l.itemsMap {
		if l.removeFn != nil {
			l.removeFn(k, l.itemsMap[k])
		}
		delete(l.itemsMap, k)
	}
	l.items.Init()
}

func (l *LRU) Size() int64 {
	return l.currentSize
}

func (l *LRU) RemoveOldest() (interface{}, bool) {
	ele := l.items.Back()
	if ele != nil {
		l.removeElement(ele)
		return ele.Value, true
	}
	return nil, false
}

func (l *LRU) lookupItem(key string) (*list.Element, bool) {
	ele, ok := l.itemsMap[key]
	return ele, ok
}

// addElement adds a new item to the cache. It does not perform any
// size checks.
func (l *LRU) addItem(key string, value interface{}, front bool) {
	var element *list.Element
	ent := &Entry{
		Key:   key,
		Value: value,
	}
	if front {
		element = l.items.PushFront(ent)
	} else {
		element = l.items.PushBack(ent)
	}
	l.itemsMap[key] = element
	l.currentSize += l.sizeFn(key, value)
}

// removeOldest removes the oldest item from the cache.
func (l *LRU) removeOldest() {
	val := l.items.Back()
	if val != nil {
		l.removeElement(val)
	}
}

// removeElement is used to remove a given list element from the cache
func (l *LRU) removeElement(e *list.Element) {
	l.items.Remove(e)
	ent := e.Value.(*Entry)
	l.currentSize -= l.sizeFn(ent.Key, ent.Value)
	for k, ele := range l.itemsMap {
		if ele == e {
			delete(l.itemsMap, k)
		}
	}
	if l.removeFn != nil {
		l.removeFn(ent.Key, ent.Value)
	}
}

var _ interfaces.LRU = (*LRU)(nil)
