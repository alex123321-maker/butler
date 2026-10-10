// Package audio accepts a small explicit audio format without executing codecs.
package audio

import (
	"encoding/binary"
	"errors"
)

var ErrFormat = errors.New("bounded mono PCM WAV required")

const Rate = 16000
const MaxSeconds = 120
const MaxBytes = 44 + Rate*2*MaxSeconds

type PCM struct {
	Samples []byte
	Rate    uint32
}

// Decode accepts only the canonical 44-byte PCM16 mono RIFF representation.
// Unknown chunks, compressed audio, lengths/trailing data and resampling are
// rejected; a caller must explicitly convert external codecs before upload.
func Decode(wav []byte, rate uint32, maxSeconds uint32) (PCM, error) {
	if len(wav) < 44 || maxSeconds == 0 || maxSeconds > MaxSeconds || (rate != 16000 && rate != 24000) ||
		string(wav[:4]) != "RIFF" || string(wav[8:16]) != "WAVEfmt " || string(wav[36:40]) != "data" ||
		binary.LittleEndian.Uint32(wav[4:8]) != uint32(len(wav)-8) || binary.LittleEndian.Uint32(wav[16:20]) != 16 ||
		binary.LittleEndian.Uint16(wav[20:22]) != 1 || binary.LittleEndian.Uint16(wav[22:24]) != 1 ||
		binary.LittleEndian.Uint32(wav[24:28]) != rate || binary.LittleEndian.Uint32(wav[28:32]) != rate*2 ||
		binary.LittleEndian.Uint16(wav[32:34]) != 2 || binary.LittleEndian.Uint16(wav[34:36]) != 16 ||
		binary.LittleEndian.Uint32(wav[40:44]) != uint32(len(wav)-44) || (len(wav)-44)%2 != 0 ||
		len(wav) == 44 || uint64(len(wav)-44) > uint64(rate)*2*uint64(maxSeconds) {
		return PCM{}, ErrFormat
	}
	return PCM{wav[44:], rate}, nil
}
func Encode(samples []byte, rate uint32) ([]byte, error) {
	if len(samples) == 0 || len(samples)%2 != 0 || (rate != 16000 && rate != 24000) || len(samples) > MaxSeconds*int(rate)*2 {
		return nil, ErrFormat
	}
	out := make([]byte, 44+len(samples))
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	copy(out[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)
	binary.LittleEndian.PutUint16(out[20:22], 1)
	binary.LittleEndian.PutUint16(out[22:24], 1)
	binary.LittleEndian.PutUint32(out[24:28], rate)
	binary.LittleEndian.PutUint32(out[28:32], rate*2)
	binary.LittleEndian.PutUint16(out[32:34], 2)
	binary.LittleEndian.PutUint16(out[34:36], 16)
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(len(samples)))
	copy(out[44:], samples)
	return out, nil
}
func (p PCM) Segment(start, end uint32) ([]byte, error) {
	if start >= end || uint64(end)*2 > uint64(len(p.Samples)) {
		return nil, ErrFormat
	}
	return Encode(p.Samples[start*2:end*2], p.Rate)
}
