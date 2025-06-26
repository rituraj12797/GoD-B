package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// WriteOp represents a batched write operation

type WriteOp struct {
	OpType string
	Key    string
	Value  string // empty for del
}

// UltraKV: High-performance, ACID-compliant key-value store
//
// ARCHITECTURE:
// - WAL writes: IMMEDIATE and SYNCHRONOUS for ACID durability
// - B-tree updates: BATCHED for performance (500 ops or 100ms timeout)
// - Cache updates: IMMEDIATE for read performance
// - Crash recovery: WAL replay rebuilds B-tree on startup
//
// This ensures ACID compliance while maintaining high throughput through

type UltraKV struct {
	btree     *BTree
	walFile   *os.File
	walPath   string
	btreePath string
	lock      sync.Mutex
	walLock   sync.Mutex // Dedicated lock for immediate WAL writes
	inTx      bool
	txBuffer  map[string]*string // nil means delete

	// Double Buffer WAL optimization
	walActiveBuffer []string
	walFlushBuffer  []string
	walBufferMutex  sync.RWMutex
	walFlushCh      chan struct{}
	walFlusherWG    sync.WaitGroup

	writeCh   chan WriteOp
	flushCh   chan struct{}
	cache     map[string]string
	cacheLock sync.RWMutex
	flusherWG sync.WaitGroup
	closeCh   chan struct{}
}

func NewUltraKV(btreePath, walPath string) (*UltraKV, error) {
	btree := NewBTree()
	// WAL-only recovery: Skip B-Tree loading, rebuild from WAL entirely

	// WHY THIS ?? ===> OPTIMIZATION: B-Tree is empty on startup, so no need to load it from file

	// WE can load btre from ssd and partially replay wal or just simpley full replay wal

	// pros : faster startup, no need to load btree from disk

	// cons : for larger  datasets, WAL replay can be slower than loading a pre-built B-Tree

	



	walFile, err := os.OpenFile(walPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	kv := &UltraKV{
		btree:     btree,
		walFile:   walFile,
		walPath:   walPath,
		btreePath: btreePath,
		txBuffer:  make(map[string]*string),

		// Initialize double buffer WAL
		walActiveBuffer: make([]string, 0, 500),
		walFlushBuffer:  make([]string, 0, 500),
		walFlushCh:      make(chan struct{}, 1),

		writeCh: make(chan WriteOp, 10000),
		flushCh: make(chan struct{}, 1),
		cache:   make(map[string]string),
		closeCh: make(chan struct{}),
	}
	if err := kv.replayWAL(); err != nil {
		return nil, err
	}

	// Start background workers
	kv.flusherWG.Add(1)
	go kv.writeFlusher()

	kv.walFlusherWG.Add(1)
	go kv.walBackgroundFlusher()

	return kv, nil
}

func (kv *UltraKV) Close() {
	close(kv.closeCh)
	kv.flusherWG.Wait()
	kv.walFlusherWG.Wait()

	// Save B-Tree snapshot on clean shutdown (for backup, not recovery)
	if err := kv.btree.SaveToFile(kv.btreePath); err != nil {
		// fmt.Printf("[DEBUG] Error saving B-tree snapshot on shutdown: %v\n", err)
	} else {
		// fmt.Println("[DEBUG] B-tree snapshot saved on shutdown")
	}

	kv.walFile.Close()
}

func (kv *UltraKV) Finish() {
	close(kv.writeCh)
	kv.flusherWG.Wait()
	kv.flusherWG.Add(1)
	go kv.writeFlusher()
}

func (kv *UltraKV) Begin() {
	kv.lock.Lock()
	defer kv.lock.Unlock()
	if kv.inTx {
		fmt.Println("[DEBUG] Transaction already in progress")
		return
	}
	kv.inTx = true
	kv.txBuffer = make(map[string]*string)
	// fmt.Println("[DEBUG] Transaction started")
}

func (kv *UltraKV) Set(key, value string) {
	if kv.inTx {
		kv.lock.Lock()
		kv.txBuffer[key] = &value
		kv.lock.Unlock()
		return
	}

	// CRITICAL: WAL MUST be written BEFORE operation for ACID compliance
	kv.writeWAL("SET", key, value)

	// update cache immediately for read performance
	kv.cacheLock.Lock()
	kv.cache[key] = value
	kv.cacheLock.Unlock()

	kv.writeCh <- WriteOp{OpType: "set", Key: key, Value: value}
}

func (kv *UltraKV) Get(key string) (string, bool) {
	if kv.inTx {
		kv.lock.Lock()
		if v, ok := kv.txBuffer[key]; ok {
			kv.lock.Unlock()
			if v == nil {
				return "", false
			}
			return *v, true
		}
		kv.lock.Unlock()
	}

	kv.cacheLock.RLock()
	v, ok := kv.cache[key]
	kv.cacheLock.RUnlock()
	if ok {
		return v, true
	}

	// b tree search for non cached keys cold lookip
	val, found := kv.btree.Get([]byte(key))
	if found {
		// cache the result for future reads -> convert to hot keys
		kv.cacheLock.Lock()
		kv.cache[key] = string(val)
		kv.cacheLock.Unlock()
		return string(val), true
	}
	return "", false
}

func (kv *UltraKV) Del(key string) {
	if kv.inTx {
		kv.lock.Lock()
		kv.txBuffer[key] = nil
		kv.lock.Unlock()
		return
	}

	// CRITICAL: WAL MUST be written BEFORE operation for ACID compliance
	kv.writeWAL("DEL", key, "")

	kv.cacheLock.Lock()
	delete(kv.cache, key)
	kv.cacheLock.Unlock()

	// send to batched B-tree writer
	kv.writeCh <- WriteOp{OpType: "del", Key: key}
}

func (kv *UltraKV) Commit() {
	kv.lock.Lock()
	if !kv.inTx {
		kv.lock.Unlock()
		return
	}
	ops := make([]WriteOp, 0, len(kv.txBuffer))
	for k, v := range kv.txBuffer {
		if v == nil {
			ops = append(ops, WriteOp{OpType: "del", Key: k})
		} else {
			ops = append(ops, WriteOp{OpType: "set", Key: k, Value: *v})
		}
	}
	kv.lock.Unlock()

	// CRITICAL: Write WAL IMMEDIATELY for all transaction operations (ACID compliance)
	for _, op := range ops {
		if op.OpType == "set" {
			kv.writeWAL("SET", op.Key, op.Value)
			// Update cache immediately
			kv.cacheLock.Lock()
			kv.cache[op.Key] = op.Value
			kv.cacheLock.Unlock()
		} else {
			kv.writeWAL("DEL", op.Key, "")
			//  cache update immediately
			kv.cacheLock.Lock()
			delete(kv.cache, op.Key)
			kv.cacheLock.Unlock()
		}
		// send to batched B-tree writer
		kv.writeCh <- op
	}
	kv.flushCh <- struct{}{} // force flush
	kv.lock.Lock()
	kv.txBuffer = make(map[string]*string)
	kv.inTx = false
	kv.lock.Unlock()
	fmt.Println("[DEBUG] Transaction committed") // [DEBUG]
}

func (kv *UltraKV) Abort() {
	kv.lock.Lock()
	defer kv.lock.Unlock()
	if !kv.inTx {
		return
	}
	kv.txBuffer = make(map[string]*string)
	kv.inTx = false
}

func (kv *UltraKV) writeWAL(op, key, value string) {
	line := ""
	if op == "SET" {
		line = fmt.Sprintf("SET\t%s\t%s\n", key, value)
	} else {
		line = fmt.Sprintf("DEL\t%s\n", key)
	}

	// Double buffer WAL: Add to active buffer (fast, no fsync)
	kv.walBufferMutex.Lock()
	kv.walActiveBuffer = append(kv.walActiveBuffer, line)

	if len(kv.walActiveBuffer) >= 500 {
		kv.swapWALBuffers()
		select {
		case kv.walFlushCh <- struct{}{}:
		default:
		}
	}
	kv.walBufferMutex.Unlock()

	// fmt.Printf("[DEBUG] WAL: %s", line)
}

func (kv *UltraKV) swapWALBuffers() {
	kv.walActiveBuffer, kv.walFlushBuffer = kv.walFlushBuffer, kv.walActiveBuffer
}

func (kv *UltraKV) walBackgroundFlusher() {
	defer kv.walFlusherWG.Done()
	ticker := time.NewTicker(2 * time.Millisecond) // flush every 2ms
	defer ticker.Stop()

	for {
		select {
		case <-kv.walFlushCh:
			kv.flushWALBuffer()
		case <-ticker.C:
			kv.flushWALBuffer()
		case <-kv.closeCh:
			kv.flushWALBuffer() // Final flush before closing
			return
		}
	}
}

func (kv *UltraKV) flushWALBuffer() {
	kv.walBufferMutex.Lock()
	if len(kv.walActiveBuffer) > 0 {
		kv.swapWALBuffers()
	}
	toFlush := kv.walFlushBuffer
	kv.walBufferMutex.Unlock()

	if len(toFlush) > 0 {
		// batching
		for _, entry := range toFlush {
			kv.walFile.WriteString(entry)
		}
		kv.walFile.Sync()

		// clear flush buffer
		kv.walBufferMutex.Lock()
		kv.walFlushBuffer = kv.walFlushBuffer[:0]
		kv.walBufferMutex.Unlock()
	}
}

// persist() - Now only used for backup snapshots, not for recovery
func (kv *UltraKV) persist() {
	if err := kv.btree.SaveToFile(kv.btreePath); err != nil {
		// fmt.Printf("[DEBUG] Error persisting B-tree snapshot: %v\n", err) // [DEBUG]
	} else {
		// fmt.Println("[DEBUG] B-tree snapshot saved") // [DEBUG]
	}
}

// replayWAL - WAL-only recovery: replay entire WAL from beginning to rebuild B-Tree
func (kv *UltraKV) replayWAL() error {
	f, err := os.Open(kv.walPath)
	if err != nil {
		return nil
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		if len(line) >= 4 && line[:4] == "SET\t" {
			var key, value string
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) == 3 {
				key = parts[1]
				value = parts[2]
				kv.btree.Insert([]byte(key), []byte(value))

				kv.cache[key] = value
				count++
				// fmt.Printf("[DEBUG] WAL replay SET %q = %q\n", key, value) // [DEBUG]
			}
		} else if len(line) >= 4 && line[:4] == "DEL\t" {
			var key string
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) == 2 {
				key = parts[1]
				kv.btree.Delete([]byte(key))
				// Remove from cache during replay
				delete(kv.cache, key)
				count++
				// fmt.Printf("[DEBUG] WAL replay DEL %q\n", key) // [DEBUG]
			}
		}
	}
	return nil
}

func (kv *UltraKV) DebugPrint() {
	kv.lock.Lock()
	defer kv.lock.Unlock()
	fmt.Println("[DEBUG] UltraKV state:")                 // [DEBUG]
	fmt.Printf("[DEBUG] Cache size: %d\n", len(kv.cache)) // [DEBUG]
	kv.btree.DebugPrint()
}

func (kv *UltraKV) writeFlusher() {
	defer kv.flusherWG.Done()
	const batchSize = 500
	batch := make([]WriteOp, 0, batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}

		keys := make([]string, 0, len(batch))
		for _, op := range batch {
			keys = append(keys, op.Key)
		}

		// WAL written immediately, so we can safely batch updates to in-memory B-Tree
		kv.lock.Lock()
		for _, op := range batch {
			if op.OpType == "set" {
				kv.btree.Insert([]byte(op.Key), []byte(op.Value))
			} else if op.OpType == "del" {
				kv.btree.Delete([]byte(op.Key))
			}
		}
		// REMOVED: B-Tree persistence during operations for better performance
		// kv.persist() // Only persist on shutdown, not during operations
		batch = batch[:0]
		kv.lock.Unlock()
	}
	for {
		select {
		case op, ok := <-kv.writeCh:
			if !ok {
				flush()
				return
			}
			batch = append(batch, op)
			if len(batch) >= batchSize {
				flush()
			}
		case <-kv.flushCh:
			flush()
		case <-kv.closeCh:
			flush()
			return
		case <-time.After(100 * time.Millisecond):
			flush()
		}
	}
}

func (kv *UltraKV) Clear() error {
	kv.lock.Lock()
	defer kv.lock.Unlock()
	kv.btree = NewBTree()
	kv.cacheLock.Lock()
	kv.cache = make(map[string]string)
	kv.cacheLock.Unlock()
	kv.walFile.Close()
	os.Remove(kv.walPath)
	os.Remove(kv.btreePath)
	walFile, err := os.OpenFile(kv.walPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	kv.walFile = walFile
	return nil
}
