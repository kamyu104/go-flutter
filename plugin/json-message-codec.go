package plugin

import (
	"encoding/json"
)

// JSONMessageCodec implements a MessageCodec using JSON for message encoding.
type JSONMessageCodec struct{}

var _ MessageCodec = JSONMessageCodec{}

// EncodeMessage encodes a json message to a slice of bytes.
func (j JSONMessageCodec) EncodeMessage(message interface{}) (binaryMessage []byte, err error) {
	return json.Marshal(&message)
}

// DecodeMessage decodes a slice of bytes to a json message.
func (j JSONMessageCodec) DecodeMessage(binaryMessage []byte) (message interface{}, err error) {
	return json.Marshal([]interface{}{binaryMessage})
}
