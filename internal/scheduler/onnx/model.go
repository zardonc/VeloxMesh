package onnx

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"

	"google.golang.org/protobuf/encoding/protowire"
)

const (
	onnxModelGraphField     protowire.Number = 7
	onnxGraphNodeField      protowire.Number = 1
	onnxNodeOutputField     protowire.Number = 2
	onnxNodeOpTypeField     protowire.Number = 4
	onnxNodeAttributeField  protowire.Number = 5
	onnxAttrNameField       protowire.Number = 1
	onnxAttrTensorField     protowire.Number = 5
	onnxTensorDataTypeField protowire.Number = 2
	onnxTensorFloatField    protowire.Number = 4
	onnxTensorRawDataField  protowire.Number = 9
	onnxTensorFloatType                      = 1
)

type modelRunner interface {
	P70OutputTokens() float64
}

type constantModelRunner struct {
	p70OutputTokens float64
}

func (r constantModelRunner) P70OutputTokens() float64 {
	return r.p70OutputTokens
}

func loadModelRunner(path string) (modelRunner, error) {
	// ponytail: supports current constant ONNX artifacts; swap in ONNX Runtime when training emits non-constant graphs.
	value, err := readConstantONNXOutput(path, "p70_output_tokens")
	if err != nil {
		return nil, err
	}
	return constantModelRunner{p70OutputTokens: value}, nil
}

func readConstantONNXOutput(path string, output string) (float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read ONNX model: %w", err)
	}
	value, ok := parseONNXModel(data, output)
	if !ok {
		return 0, fmt.Errorf("ONNX model does not expose constant output %q", output)
	}
	return value, nil
}

func parseONNXModel(data []byte, output string) (float64, bool) {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return 0, false
		}
		data = data[n:]
		value, m := consumeField(num, typ, data)
		if m < 0 {
			return 0, false
		}
		if num == onnxModelGraphField && typ == protowire.BytesType {
			if got, ok := parseONNXGraph(value, output); ok {
				return got, true
			}
		}
		data = data[m:]
	}
	return 0, false
}

func parseONNXGraph(data []byte, output string) (float64, bool) {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return 0, false
		}
		data = data[n:]
		value, m := consumeField(num, typ, data)
		if m < 0 {
			return 0, false
		}
		if num == onnxGraphNodeField && typ == protowire.BytesType {
			if got, ok := parseONNXNode(value, output); ok {
				return got, true
			}
		}
		data = data[m:]
	}
	return 0, false
}

func parseONNXNode(data []byte, output string) (float64, bool) {
	opType := ""
	outputMatched := false
	value := 0.0
	valueFound := false
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return 0, false
		}
		data = data[n:]
		field, m := consumeField(num, typ, data)
		if m < 0 {
			return 0, false
		}
		switch num {
		case onnxNodeOpTypeField:
			opType = string(field)
		case onnxNodeOutputField:
			outputMatched = outputMatched || string(field) == output
		case onnxNodeAttributeField:
			if got, ok := parseONNXAttribute(field); ok {
				value = got
				valueFound = true
			}
		}
		data = data[m:]
	}
	return value, opType == "Constant" && outputMatched && valueFound
}

func parseONNXAttribute(data []byte) (float64, bool) {
	name := ""
	value := 0.0
	valueFound := false
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return 0, false
		}
		data = data[n:]
		field, m := consumeField(num, typ, data)
		if m < 0 {
			return 0, false
		}
		if num == onnxAttrNameField {
			name = string(field)
		}
		if num == onnxAttrTensorField && typ == protowire.BytesType {
			value, valueFound = parseONNXTensor(field)
		}
		data = data[m:]
	}
	return value, name == "value" && valueFound
}

func parseONNXTensor(data []byte) (float64, bool) {
	dataType := int64(0)
	value := 0.0
	valueFound := false
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return 0, false
		}
		data = data[n:]
		field, m := consumeField(num, typ, data)
		if m < 0 {
			return 0, false
		}
		if num == onnxTensorDataTypeField && typ == protowire.VarintType {
			got, _ := protowire.ConsumeVarint(field)
			dataType = int64(got)
		}
		if num == onnxTensorFloatField {
			value, valueFound = parseONNXFloatField(typ, field)
		}
		if num == onnxTensorRawDataField && len(field) >= 4 {
			value = float64(math.Float32frombits(binary.LittleEndian.Uint32(field[:4])))
			valueFound = true
		}
		data = data[m:]
	}
	return value, dataType == onnxTensorFloatType && valueFound
}

func parseONNXFloatField(typ protowire.Type, data []byte) (float64, bool) {
	if typ == protowire.Fixed32Type && len(data) == 4 {
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(data))), true
	}
	if typ != protowire.BytesType || len(data) < 4 {
		return 0, false
	}
	return float64(math.Float32frombits(binary.LittleEndian.Uint32(data[:4]))), true
}

func consumeField(num protowire.Number, typ protowire.Type, data []byte) ([]byte, int) {
	if typ == protowire.BytesType {
		value, n := protowire.ConsumeBytes(data)
		return value, n
	}
	n := protowire.ConsumeFieldValue(num, typ, data)
	if n < 0 {
		return nil, n
	}
	return data[:n], n
}
