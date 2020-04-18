package util

import (
	"bytes"
	"encoding/gob"
)

func DeepCopyInto(to interface{}, from interface{}) {
	buff := new(bytes.Buffer)
	enc := gob.NewEncoder(buff)
	dec := gob.NewDecoder(buff)
	_ = enc.Encode(from)
	_ = dec.Decode(to)
}
