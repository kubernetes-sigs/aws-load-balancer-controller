package quic

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
)

type mockWorkerIdGenerator struct {
	id  int64
	err error
}

func (m *mockWorkerIdGenerator) getWorkerId(maxId int64) (int64, error) {
	return m.id, m.err
}

func TestNewQuicServerIDGenerator(t *testing.T) {
	serverIdGenerator := newQuicServerIDGenerator(&mockWorkerIdGenerator{
		id: 100,
	})

	id, err := serverIdGenerator.generate()
	assert.NoError(t, err)

	bs, err := base64.StdEncoding.DecodeString(id)
	assert.NoError(t, err)
	assert.Equal(t, 8, len(bs))
}

func TestNewQuicServerIDGenerator_initialNoError(t *testing.T) {
	mockWorkerIdGen := &mockWorkerIdGenerator{
		id:  100,
		err: nil,
	}
	serverIdGenerator := newQuicServerIDGenerator(mockWorkerIdGen)

	id, err := serverIdGenerator.generate()
	assert.NoError(t, err)

	bs, err := base64.StdEncoding.DecodeString(id)
	assert.NoError(t, err)
	assert.Equal(t, 8, len(bs))

	// run again, should get a different result
	id2, err := serverIdGenerator.generate()
	assert.NoError(t, err)
	assert.NotEqual(t, id, id2)
	bs2, err := base64.StdEncoding.DecodeString(id2)
	assert.NoError(t, err)
	assert.Equal(t, 8, len(bs2))

	// workerId generating an error shouldn't cause an issue as we cache the worker id
	mockWorkerIdGen.err = fmt.Errorf("bad thing")
	id3, err := serverIdGenerator.generate()
	assert.NoError(t, err)
	assert.NotEqual(t, id, id3)
	assert.NotEqual(t, id2, id3)
	bs3, err := base64.StdEncoding.DecodeString(id3)
	assert.NoError(t, err)
	assert.Equal(t, 8, len(bs3))

	// using a new server id gen should lead to an error as the worker id is not cached
	serverIdGenerator2 := newQuicServerIDGenerator(mockWorkerIdGen)
	_, err = serverIdGenerator2.generate()
	assert.Error(t, err)
}

func TestQuicServerIDGenerator_basicGenerate(t *testing.T) {
	workerId := int64(100)
	timestamp := epoch + int64(500)
	randValue := int64(5)

	serverIdGenerator := quicServerIDGeneratorImpl{
		mutex: sync.Mutex{},
		workerIdGen: &mockWorkerIdGenerator{
			id: workerId,
		},
		currentTimestampFn: func() int64 {
			return timestamp
		},
		randFn: func(i int64) int64 {
			return randValue
		},
	}

	id, err := serverIdGenerator.generate()
	assert.NoError(t, err)
	bs, err := base64.StdEncoding.DecodeString(id)
	generatedTs, generatedWid, generatedSeq := parseSnowflake(int64(binary.BigEndian.Uint64(bs)))
	assert.Equal(t, workerId, generatedWid)
	assert.Equal(t, timestamp, generatedTs)
	assert.Equal(t, randValue, generatedSeq)
}

func TestQuicServerIDGenerator_multipleGeneratesOnSameTimestamp(t *testing.T) {
	workerId := int64(100)
	timestamp := epoch + int64(500)
	randValue := int64(5)

	serverIdGenerator := quicServerIDGeneratorImpl{
		mutex: sync.Mutex{},
		workerIdGen: &mockWorkerIdGenerator{
			id: workerId,
		},
		currentTimestampFn: func() int64 {
			return timestamp
		},
		randFn: func(i int64) int64 {
			return randValue
		},
	}

	// First generation -- same ts
	id, err := serverIdGenerator.generate()
	assert.NoError(t, err)
	bs, err := base64.StdEncoding.DecodeString(id)
	assert.NoError(t, err)
	generatedTs, generatedWid, generatedSeq := parseSnowflake(int64(binary.BigEndian.Uint64(bs)))
	assert.Equal(t, workerId, generatedWid)
	assert.Equal(t, timestamp, generatedTs)
	assert.Equal(t, randValue, generatedSeq)

	// Second generation -- same ts
	id2, err := serverIdGenerator.generate()
	assert.NoError(t, err)
	bs2, err := base64.StdEncoding.DecodeString(id2)
	assert.NoError(t, err)
	generatedTs2, generatedWid2, generatedSeq2 := parseSnowflake(int64(binary.BigEndian.Uint64(bs2)))
	assert.Equal(t, workerId, generatedWid2)
	assert.Equal(t, timestamp, generatedTs2)
	assert.Equal(t, randValue+1, generatedSeq2)

	// Third generation -- same ts
	id3, err := serverIdGenerator.generate()
	assert.NoError(t, err)
	bs3, err := base64.StdEncoding.DecodeString(id3)
	assert.NoError(t, err)
	generatedTs3, generatedWid3, generatedSeq3 := parseSnowflake(int64(binary.BigEndian.Uint64(bs3)))
	assert.Equal(t, workerId, generatedWid3)
	assert.Equal(t, timestamp, generatedTs3)
	assert.Equal(t, randValue+2, generatedSeq3)

	// Move ts forward, use new rand value
	newTs := epoch + int64(1000)
	newSeq := int64(100)
	serverIdGenerator.currentTimestampFn = func() int64 {
		return newTs
	}
	serverIdGenerator.randFn = func(i int64) int64 {
		return newSeq
	}

	id4, err := serverIdGenerator.generate()
	assert.NoError(t, err)
	bs4, err := base64.StdEncoding.DecodeString(id4)
	assert.NoError(t, err)
	generatedTs4, generatedWid4, generatedSeq4 := parseSnowflake(int64(binary.BigEndian.Uint64(bs4)))
	assert.Equal(t, workerId, generatedWid4)
	assert.Equal(t, newTs, generatedTs4)
	assert.Equal(t, newSeq, generatedSeq4)
}

func TestQuicServerIDGenerator_sequenceRollover(t *testing.T) {
	workerId := int64(100)
	timestamp := epoch + int64(500)
	randValue := int64(maxSequence)

	counter := int64(0)

	randCalled := false

	serverIdGenerator := quicServerIDGeneratorImpl{
		mutex: sync.Mutex{},
		workerIdGen: &mockWorkerIdGenerator{
			id: workerId,
		},
		currentTimestampFn: func() int64 {
			nts := timestamp + counter
			counter++
			return nts
		},
		randFn: func(i int64) int64 {
			if randCalled {
				return int64(5)
			}
			randCalled = true
			return randValue
		},
	}

	// First generation -- a sequence hasn't rolled over
	id, err := serverIdGenerator.generate()
	assert.NoError(t, err)
	bs, err := base64.StdEncoding.DecodeString(id)
	assert.NoError(t, err)
	generatedTs, generatedWid, generatedSeq := parseSnowflake(int64(binary.BigEndian.Uint64(bs)))
	assert.Equal(t, workerId, generatedWid)
	assert.Equal(t, timestamp, generatedTs)
	assert.Equal(t, randValue, generatedSeq)

	// Second generation -- a sequence has rolled over
	id2, err := serverIdGenerator.generate()
	assert.NoError(t, err)
	bs2, err := base64.StdEncoding.DecodeString(id2)
	assert.NoError(t, err)
	generatedTs2, generatedWid2, generatedSeq2 := parseSnowflake(int64(binary.BigEndian.Uint64(bs2)))
	assert.Equal(t, workerId, generatedWid2)
	assert.Equal(t, timestamp+1, generatedTs2)
	assert.Equal(t, int64(5), generatedSeq2)
}

func TestServerIdUniqueness(t *testing.T) {
	numThreads := 5
	gen1 := newQuicServerIDGenerator(&mockWorkerIdGenerator{
		id: 1,
	})
	gen2 := newQuicServerIDGenerator(&mockWorkerIdGenerator{
		id: 2,
	})
	gen3 := newQuicServerIDGenerator(&mockWorkerIdGenerator{
		id: 3,
	})
	gen4 := newQuicServerIDGenerator(&mockWorkerIdGenerator{
		id: 4,
	})
	gen5 := newQuicServerIDGenerator(&mockWorkerIdGenerator{
		id: 5,
	})

	mutex := sync.Mutex{}
	set := make(map[string]bool)

	iterations := 100000
	workerThread := func(local quicServerIDGenerator) {
		for i := 0; i < iterations; i++ {
			id, err := local.generate()
			assert.NoError(t, err)
			mutex.Lock()
			set[id] = true
			mutex.Unlock()
		}
	}

	wg := sync.WaitGroup{}
	wg.Add(numThreads)
	go func() {
		workerThread(gen1)
		wg.Done()
	}()
	go func() {
		workerThread(gen2)
		wg.Done()
	}()
	go func() {
		workerThread(gen3)
		wg.Done()
	}()
	go func() {
		workerThread(gen4)
		wg.Done()
	}()
	go func() {
		workerThread(gen5)
		wg.Done()
	}()
	wg.Wait()
	// We expect no duplicates, so expect that every iteration in each thread produced a distinct result
	assert.Equal(t, iterations*numThreads, len(set))
}

// parseSnowflake extracts components from a Snowflake ID
func parseSnowflake(id int64) (timestamp int64, workerID int64, sequence int64) {
	timestamp = (id >> timestampShift) + epoch
	workerID = (id >> workerIDShift) & maxWorkerID
	sequence = id & maxSequence
	return
}
