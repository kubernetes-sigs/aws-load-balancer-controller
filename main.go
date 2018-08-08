package main

import (
	"fmt"
	"reflect"
)

type custom string
type SVC struct {
	Value *custom
}

func (s *SVC) SetField(field string, v interface{}) {
	reflect.ValueOf(s).Elem().FieldByName(field).Set(reflect.ValueOf(v))
}

func main() {
	str := custom("str")
	newstr := custom("new str")
	s := &SVC{Value: &str}
	fmt.Println(*s.Value)
	s.SetField("Value", &newstr)
	fmt.Println(*s.Value)
}
