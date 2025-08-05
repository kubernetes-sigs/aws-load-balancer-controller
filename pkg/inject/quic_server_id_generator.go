package inject

import (
	"encoding/base64"
	"encoding/binary"
	"sync"
	"time"
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
	// TODO - Change this back once I've fixed envoy.
	return base64.StdEncoding.EncodeToString(bs[:6])
}

func newQuicServerIDGenerator() quicServerIDGenerator {
	return &quicServerIDGeneratorImpl{}
}
