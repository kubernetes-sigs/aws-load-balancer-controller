package quic

import (
	"encoding/base64"
	"encoding/binary"
	"sync"
	"time"
)

const (
	// Epoch start time (January 1, 2020 00:00:00 UTC)
	// You can adjust this to your application's start date
	epoch int64 = 1577836800000 // milliseconds

	/*
		• **39 bits timestamp** (17.4 years / roll over)
		• **18 bits worker ID** (262,144 workers -> small chance of collision)
		• **7 bits sequence** (128 pod launches per ms)
	*/

	// Bit lengths
	workerIDBits  = 18
	sequenceBits  = 7
	timestampBits = 39

	// Maximum values
	maxWorkerID = (1 << workerIDBits) - 1
	maxSequence = (1 << sequenceBits) - 1 // 4095
	// Bit shifts
	workerIDShift  = sequenceBits
	timestampShift = sequenceBits + workerIDBits
)

type quicServerIDGenerator interface {
	generate() string
}

type quicServerIDGeneratorImpl struct {
	mutex sync.Mutex
}

func (q *quicServerIDGeneratorImpl) generate() string {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	bs := make([]byte, 8)
	binary.BigEndian.PutUint64(bs, uint64(time.Now().UnixMilli()))
	return base64.StdEncoding.EncodeToString(bs)
}

func newQuicServerIDGenerator() quicServerIDGenerator {
	return &quicServerIDGeneratorImpl{}
}
