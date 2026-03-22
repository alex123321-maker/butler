package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

func ReadRequest(r io.Reader) (Request, error) {
	payload, err := readFrame(r)
	if err != nil {
		return Request{}, err
	}
	var request Request
	if err := json.Unmarshal(payload, &request); err != nil {
		return Request{}, fmt.Errorf("decode native message request: %w", err)
	}
	return request, nil
}

func ReadMessage(r io.Reader) (any, error) {
	payload, err := readFrame(r)
	if err != nil {
		return nil, err
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("decode native message envelope: %w", err)
	}
	if _, ok := envelope["method"]; ok {
		var request Request
		if err := json.Unmarshal(payload, &request); err != nil {
			return nil, fmt.Errorf("decode native message request: %w", err)
		}
		return request, nil
	}

	var response Response
	if err := json.Unmarshal(payload, &response); err != nil {
		return nil, fmt.Errorf("decode native message response: %w", err)
	}
	return response, nil
}

func WriteRequest(w io.Writer, request Request) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode native message request: %w", err)
	}
	return writeFrame(w, payload)
}

func WriteResponse(w io.Writer, response Response) error {
	payload, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encode native message response: %w", err)
	}
	return writeFrame(w, payload)
}

func readFrame(r io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return nil, err
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func writeFrame(w io.Writer, payload []byte) error {
	if err := binary.Write(w, binary.LittleEndian, uint32(len(payload))); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}
