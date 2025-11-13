package quic

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

const (
	// Epoch start time (January 1, 2020 00:00:00 UTC)
	// You can adjust this to your application's start date
	epoch int64 = 1577836800000 // milliseconds

	/*
		• **39 bits timestamp** (17.4 years / roll over)
		• **17 bits worker ID** (131,071 workers -> small chance of collision)
		• **7 bits sequence** (128 pod launches per ms)
	*/

	// Bit lengths
	workerIDBits  = 17
	sequenceBits  = 7
	timestampBits = 39

	// Maximum values
	maxWorkerID = (1 << workerIDBits) - 1
	maxSequence = (1 << sequenceBits) - 1
	// Bit shifts
	workerIDShift  = sequenceBits
	timestampShift = sequenceBits + workerIDBits
)

func newQuicServerIDGenerator(workerIDGen workerIdGenerator) quicServerIDGenerator {
	return &quicServerIDGeneratorImpl{
		mutex:       sync.Mutex{},
		workerIdGen: workerIDGen,

		sequence:      0,
		lastTimestamp: 0,
		currentTimestampFn: func() int64 {
			return time.Now().UnixMilli()
		},
		randFn: func(max int64) int64 {
			return rand.Int63n(max)
		},
	}
}

type quicServerIDGenerator interface {
	generate() (string, error)
}

type quicServerIDGeneratorImpl struct {
	mutex sync.Mutex

	// Dynamically assigned on first allocation
	workerId *int64
	// Worker ID generation
	workerIdGen workerIdGenerator

	// Updated on each ID generation.
	sequence      int64
	lastTimestamp int64

	// Stuff that is mocked out for unit tests
	currentTimestampFn func() int64
	randFn             func(int64) int64
}

func (q *quicServerIDGeneratorImpl) generate() (string, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.workerId == nil {
		err := q.provisionWorkerId()
		if err != nil {
			return "", err
		}
	}

	timestamp := q.currentTimestampFn()
	if timestamp < q.lastTimestamp {
		return "", fmt.Errorf("clock moved backwards, refusing to generate ID")
	}

	newSequenceRequested := true
	// If same millisecond, increment sequence
	if timestamp == q.lastTimestamp {
		q.sequence = (q.sequence + 1) & maxSequence

		// Sequence overflow - wait for next millisecond
		if q.sequence == 0 {
			time.Sleep(2 * time.Millisecond)
			timestamp = q.currentTimestampFn()
		} else {
			newSequenceRequested = false
		}
	}

	if newSequenceRequested {
		q.sequence = q.randFn(maxSequence)
	}

	q.lastTimestamp = timestamp
	id := ((timestamp - epoch) << timestampShift) |
		(*q.workerId << workerIDShift) |
		q.sequence

	bs := make([]byte, 8)
	binary.BigEndian.PutUint64(bs, uint64(id))
	return base64.StdEncoding.EncodeToString(bs), nil
}

func (q *quicServerIDGeneratorImpl) provisionWorkerId() error {
	workerId, err := q.workerIdGen.getWorkerId(maxWorkerID)

	if err != nil {
		return err
	}
	q.workerId = &workerId
	return nil
}
