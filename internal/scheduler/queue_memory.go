package scheduler

import (
	"container/heap"
	"context"
	"sort"
	"sync"
)

type MemoryQueue struct {
	mu    sync.Mutex
	items memoryHeap
	index map[string]*memoryItem
	next  int64
}

func NewMemoryQueue() *MemoryQueue {
	q := &MemoryQueue{index: map[string]*memoryItem{}}
	heap.Init(&q.items)
	return q
}

func (q *MemoryQueue) Push(_ context.Context, item QueueItem) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if existing, ok := q.index[item.TaskID]; ok {
		existing.QueueItem = item
		heap.Fix(&q.items, existing.index)
		return nil
	}
	q.next++
	wrapped := &memoryItem{QueueItem: item, seq: q.next}
	heap.Push(&q.items, wrapped)
	q.index[item.TaskID] = wrapped
	return nil
}

func (q *MemoryQueue) PeekMin(_ context.Context, limit int) ([]QueueItem, error) {
	if limit < 1 {
		return []QueueItem{}, nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if limit < len(q.items) {
		return q.peekMinBounded(limit), nil
	}
	copied := make(memoryHeap, len(q.items))
	for i, item := range q.items {
		cloned := *item
		cloned.index = i
		copied[i] = &cloned
	}
	heap.Init(&copied)
	items := make([]QueueItem, 0, min(limit, copied.Len()))
	for copied.Len() > 0 && len(items) < limit {
		item := heap.Pop(&copied).(*memoryItem)
		items = append(items, item.QueueItem)
	}
	return items, nil
}

func (q *MemoryQueue) peekMinBounded(limit int) []QueueItem {
	best := make([]*memoryItem, 0, limit)
	for _, item := range q.items {
		insertAt := sort.Search(len(best), func(i int) bool {
			return memoryItemLess(item, best[i])
		})
		if insertAt >= limit {
			continue
		}
		best = append(best, nil)
		copy(best[insertAt+1:], best[insertAt:])
		best[insertAt] = item
		if len(best) > limit {
			best = best[:limit]
		}
	}
	items := make([]QueueItem, len(best))
	for i, item := range best {
		items[i] = item.QueueItem
	}
	return items
}

func (q *MemoryQueue) PopMin(_ context.Context) (QueueItem, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.items.Len() == 0 {
		return QueueItem{}, ErrQueueEmpty
	}
	item := heap.Pop(&q.items).(*memoryItem)
	delete(q.index, item.TaskID)
	return item.QueueItem, nil
}

func (q *MemoryQueue) Remove(_ context.Context, taskID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	item, ok := q.index[taskID]
	if !ok {
		return ErrTaskNotFound
	}
	heap.Remove(&q.items, item.index)
	delete(q.index, taskID)
	return nil
}

func (q *MemoryQueue) Len(_ context.Context) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return int64(q.items.Len()), nil
}

type memoryItem struct {
	QueueItem
	seq   int64
	index int
}

type memoryHeap []*memoryItem

func (h memoryHeap) Len() int { return len(h) }

func (h memoryHeap) Less(i, j int) bool {
	return memoryItemLess(h[i], h[j])
}

func memoryItemLess(left, right *memoryItem) bool {
	if left.Score == right.Score {
		return left.seq < right.seq
	}
	return left.Score < right.Score
}

func (h memoryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *memoryHeap) Push(x any) {
	item := x.(*memoryItem)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *memoryHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	item.index = -1
	*h = old[:n-1]
	return item
}
