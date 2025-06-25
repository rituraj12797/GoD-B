package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"
)

const (
	numPairs  = 100000
	valueSize = 100

	// Test database paths
	testBtreePath = "benchmark.db"
	testWalPath   = "benchmark.db.wal"
)

func generateRandomValue(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func cleanup() {
	os.Remove(testBtreePath)
	os.Remove(testWalPath)
}

func BenchmarkKVStoreCreation(b *testing.B) {
	cleanup()

	b.ResetTimer()
	start := time.Now()

	kv, err := NewUltraKV(testBtreePath, testWalPath)
	if err != nil {
		b.Fatalf("Failed to create KV store: %v", err)
	}
	defer kv.Close()

	elapsed := time.Since(start)
	opsPerSecond := float64(1) / elapsed.Seconds()

	fmt.Printf("KV Store Creation: %.2f ops/sec (%.6f seconds)\n", opsPerSecond, elapsed.Seconds())
}
func BenchmarkKVStoreWrites(b *testing.B) {
	cleanup()

	kv, err := NewUltraKV(testBtreePath, testWalPath)
	if err != nil {
		b.Fatalf("Failed to create KV store: %v", err)
	}
	defer kv.Close()

	values := make([]string, numPairs)
	for i := 0; i < numPairs; i++ {
		values[i] = generateRandomValue(valueSize)
	}

	b.ResetTimer()
	start := time.Now()

	for i := 0; i < numPairs; i++ {
		key := "key" + strconv.Itoa(i)
		kv.Set(key, values[i])
	}

	kv.flushCh <- struct{}{}

	elapsed := time.Since(start)
	opsPerSecond := float64(numPairs) / elapsed.Seconds()

	fmt.Printf("Writes (%d pairs): %.2f ops/sec (%.6f seconds)\n",
		numPairs, opsPerSecond, elapsed.Seconds())
}

func BenchmarkKVStoreReads(b *testing.B) {
	cleanup()

	kv, err := NewUltraKV(testBtreePath, testWalPath)
	if err != nil {
		b.Fatalf("Failed to create KV store: %v", err)
	}

	values := make([]string, numPairs)
	for i := 0; i < numPairs; i++ {
		values[i] = generateRandomValue(valueSize)
	}

	for i := 0; i < numPairs; i++ {
		key := "key" + strconv.Itoa(i)
		kv.Set(key, values[i])
	}

	kv.flushCh <- struct{}{}
	readIndices := rand.Perm(numPairs)

	b.ResetTimer()
	start := time.Now()

	for _, i := range readIndices {
		key := "key" + strconv.Itoa(i)
		_, found := kv.Get(key)
		if !found {
			b.Fatalf("Key not found: %s", key)
		}
	}

	elapsed := time.Since(start)
	opsPerSecond := float64(numPairs) / elapsed.Seconds()

	fmt.Printf("Reads (%d pairs): %.2f ops/sec (%.6f seconds)\n",
		numPairs, opsPerSecond, elapsed.Seconds())

	kv.Close()
}

func BenchmarkKVStoreTransactionWrites(b *testing.B) {
	cleanup()

	kv, err := NewUltraKV(testBtreePath, testWalPath)
	if err != nil {
		b.Fatalf("failed to create KV store: %v", err)
	}
	defer kv.Close()

	values := make([]string, numPairs)
	for i := 0; i < numPairs; i++ {
		values[i] = generateRandomValue(valueSize)
	}

	batchSize := 1000
	numBatches := numPairs / batchSize

	b.ResetTimer()
	start := time.Now()

	for batch := 0; batch < numBatches; batch++ {
		kv.Begin()
		for i := 0; i < batchSize; i++ {
			idx := batch*batchSize + i
			key := "txkey" + strconv.Itoa(idx)
			kv.Set(key, values[idx])
		}
		kv.Commit()
	}

	elapsed := time.Since(start)
	opsPerSecond := float64(numPairs) / elapsed.Seconds()

	fmt.Printf("Transaction Writes (%d pairs): %.2f ops/sec (%.6f seconds)\n",
		numPairs, opsPerSecond, elapsed.Seconds())
}

func BenchmarkKVStoreMixedWorkload(b *testing.B) {
	cleanup()

	kv, err := NewUltraKV(testBtreePath, testWalPath)
	if err != nil {
		b.Fatalf("Failed to create KV store: %v", err)
	}
	defer kv.Close()

	values := make([]string, numPairs)
	for i := 0; i < numPairs; i++ {
		values[i] = generateRandomValue(valueSize)
	}

	halfPairs := numPairs / 2
	for i := 0; i < halfPairs; i++ {
		key := "key" + strconv.Itoa(i)
		kv.Set(key, values[i])
	}

	kv.flushCh <- struct{}{}

	b.ResetTimer()
	start := time.Now()

	totalOps := numPairs
	for i := 0; i < totalOps; i++ {
		if i%2 == 0 {
			readIdx := rand.Intn(halfPairs)
			key := "key" + strconv.Itoa(readIdx)
			_, found := kv.Get(key)
			if !found {
				b.Fatalf("Key not found: %s", key)
			}
		} else {
			writeIdx := halfPairs + (i / 2)
			key := "key" + strconv.Itoa(writeIdx)
			kv.Set(key, values[writeIdx])
		}
	}

	elapsed := time.Since(start)
	opsPerSecond := float64(totalOps) / elapsed.Seconds()
	fmt.Printf("Mixed Workload (%d ops, 50%% reads, 50%% writes): %.2f ops/sec (%.6f seconds)\n",
		totalOps, opsPerSecond, elapsed.Seconds())
}

func TestBenchmarkReport(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	fmt.Println("\n=== UltraKV Performance Benchmark Report ===")
	fmt.Printf("Testing with %d key-value pairs, %d bytes per value\n\n", numPairs, valueSize)

	b := &testing.B{}
	BenchmarkKVStoreCreation(b)
	BenchmarkKVStoreWrites(b)
	BenchmarkKVStoreReads(b)
	BenchmarkKVStoreTransactionWrites(b)
	BenchmarkKVStoreMixedWorkload(b)

	fmt.Println("\n=== Benchmark Complete ===")
	cleanup()
}
