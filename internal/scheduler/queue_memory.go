package scheduler

import (
	"container/heap"
	"context"
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
	if h[i].Score == h[j].Score {
		return h[i].seq < h[j].seq
	}
	return h[i].Score < h[j].Score
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
