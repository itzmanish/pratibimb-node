package internal

import (
	"encoding/json"
	"math/rand"

	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/jkawamoto/structpbconv"
)

func generateRandomNumber() uint32 {
	return rand.Uint32()
}

func NewBool(b bool) *bool {
	return &b
}

func Clone(dst, source interface{}) error {
	data, err := json.Marshal(source)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func PbToStruct(payload *structpb.Struct, res interface{}) error {
	return structpbconv.Convert(payload, &res)
}
