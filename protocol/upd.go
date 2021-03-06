package protocol

import (
	"encoding/binary"
	"errors"

	"github.com/Sirupsen/logrus"
)

// Operations for hamgo protocol messages.
const (
	UpdOperationCacheRequest  = 0
	UpdOperationCacheResponse = 1
)

// UpdPayload defines the payload for hamgo signaling.
type UpdPayload struct {
	Operation  uint8
	DataLength uint16
	Data       []byte
}

// UpdRequestCacheEntry is sent in a cache request message to inform the remote peer
// of the messages that are already in the cache of the node so that only new messages
// have to be sent from the remote peer.
type UpdRequestCacheEntry struct {
	SeqCounter uint64
	Source     Contact
}

// UpdPayloadCacheRequest is the request sent by a node to get missing cache messages.
type UpdPayloadCacheRequest struct {
	NumEntries uint32
	Entries    []UpdRequestCacheEntry
}

// UpdPayloadEntry contains a message for the reply message.
// This is needed as messages may corrupt and therefore the length
// may not match and a single corrupted message would otherwise
// corrupt the whole reply.
type UpdPayloadEntry struct {
	Length  uint32
	Message Message
}

// UpdPayloadCacheResponse is sent as an answer to a cache query in order to update the
// querying nodes cache.
type UpdPayloadCacheResponse struct {
	NumEntries uint32
	Entries    []UpdPayloadEntry
}

// Bytes converts an entry to bytes.
func (e *UpdPayloadEntry) Bytes() []byte {
	msb := e.Message.Bytes()
	lm := len(msb)

	buf := make([]byte, 4+lm)
	idx := 0

	binary.LittleEndian.PutUint32(buf[idx:], uint32(lm))
	idx += 4

	copy(buf[idx:], msb)
	return buf
}

// ParsePayloadEntry parses an entry and tries to fail gracefully if the
// message is corrupted.
func ParsePayloadEntry(buf []byte) (*UpdPayloadEntry, []byte) {
	re := UpdPayloadEntry{}
	idx := 0

	if len(buf) < 4 {
		logrus.Warn("Upd: Failed to parse payload entry")
		return nil, nil
	}

	re.Length = binary.LittleEndian.Uint32(buf[idx : idx+4])
	idx += 4

	if len(buf) < idx+int(re.Length) {
		logrus.Warn("Upd: Failed to parse payload entry, msg length > buf len")
		return nil, nil
	}

	msg, _ := ParseMessage(buf[idx:])
	if msg == nil {
		logrus.Warn("Upd: failed to parse message, ignoring and continuing")
		return nil, buf[idx+int(re.Length):]
	}

	re.Message = *msg
	return &re, buf[idx+int(re.Length):]
}

// Bytes converts a cache entry to bytes.
func (e *UpdRequestCacheEntry) Bytes() []byte {
	ct := e.Source.Bytes()
	buf := make([]byte, len(ct)+4)
	idx := 0

	binary.LittleEndian.PutUint64(buf[idx:idx+8], e.SeqCounter)
	idx += 8

	copy(buf[idx:], ct)
	return buf
}

// ParseCacheEntry parses a cache entry and returns the remaining buffer.
func ParseCacheEntry(buf []byte) (*UpdRequestCacheEntry, []byte) {
	re := UpdRequestCacheEntry{}
	idx := 0

	if len(buf) < 9 {
		logrus.Warn("Upd: Failed to parse cache entry, %d < %d", len(buf), 9)
		return nil, nil
	}

	re.SeqCounter = binary.LittleEndian.Uint64(buf[idx : idx+8])
	idx += 8

	ct, rbuf := ParseContact(buf[idx:])
	if ct == nil {
		logrus.Warn("Upd: failed to parse contact")
		return nil, nil
	}

	re.Source = *ct

	return &re, rbuf
}

// Bytes converts an update protocol request to a byte buffer.
func (r *UpdPayloadCacheRequest) Bytes() []byte {
	var ent [][]byte
	tlen := 0

	for _, c := range r.Entries {
		e := c.Bytes()
		tlen += len(e)
		ent = append(ent, e)
	}

	buf := make([]byte, tlen+4)
	idx := 0

	binary.LittleEndian.PutUint32(buf[idx:], r.NumEntries)
	idx += 4

	for _, e := range ent {
		// put entries
		copy(buf[idx:], e)

		idx += len(e)
	}

	return buf
}

// Bytes converts the cache response to a byte buffer.
func (r *UpdPayloadCacheResponse) Bytes() []byte {
	var mbufs [][]byte
	tlen := 0

	for _, v := range r.Entries {
		b := v.Bytes()
		mbufs = append(mbufs, b)
		tlen += len(b)
	}

	buf := make([]byte, tlen+4)
	idx := 0

	binary.LittleEndian.PutUint32(buf[idx:idx+4], r.NumEntries)
	idx += 4

	for _, e := range mbufs {
		copy(buf[idx:], e)
		idx += len(e)
	}

	return buf
}

// ParsePayloadCacheResponse parses a cache response.
func ParsePayloadCacheResponse(buf []byte) *UpdPayloadCacheResponse {
	idx := 0
	pcr := UpdPayloadCacheResponse{}

	if len(buf) < 4 {
		return nil
	}

	pcr.NumEntries = binary.LittleEndian.Uint32(buf[idx : idx+4])
	idx += 4

	for i := 0; i < int(pcr.NumEntries); i++ {
		m, rbuf := ParsePayloadEntry(buf[idx:])
		if m == nil {
			logrus.Warn("Upd: failed to parse cache response, skipping message")
			continue
		}

		pcr.Entries = append(pcr.Entries, *m)
		idx = 0
		buf = rbuf
	}

	return &pcr
}

// ParsePayloadCacheRequest parses a cache request.
func ParsePayloadCacheRequest(buf []byte) *UpdPayloadCacheRequest {
	cr := UpdPayloadCacheRequest{}
	idx := 0

	if len(buf) < 4 {
		return nil
	}

	cr.NumEntries = binary.LittleEndian.Uint32(buf[idx : idx+4])
	idx += 4

	buf = buf[idx:]
	for i := 0; i < int(cr.NumEntries); i++ {
		e, rbuf := ParseCacheEntry(buf)
		if e == nil {
			logrus.Warn("Upd: failed to parse cache request, skipping entry")
			continue
		}

		cr.Entries = append(cr.Entries, *e)

		buf = rbuf

		if len(buf) == 0 {
			break
		}
	}

	return &cr
}

// Bytes converts a update protocol payload to a byte buffer.
func (u *UpdPayload) Bytes() []byte {
	buf := make([]byte, 3+int(u.DataLength))
	idx := 0

	buf[idx] = u.Operation
	idx++

	binary.LittleEndian.PutUint16(buf[idx:idx+2], u.DataLength)
	idx += 2

	copy(buf[idx:], u.Data[:u.DataLength])
	return buf
}

// ParseUpdPayload parses an update protocol payload.
func ParseUpdPayload(buf []byte) (*UpdPayload, error) {
	upd := UpdPayload{}
	idx := 0

	if len(buf) < 3 {
		logrus.Warn("Upd: failed to parse payload")
		return nil, errors.New("payload invalid")
	}

	upd.Operation = buf[idx]
	idx++

	upd.DataLength = binary.LittleEndian.Uint16(buf[idx : idx+2])
	idx += 2

	if (len(buf) - int(idx)) < int(upd.DataLength) {
		logrus.Warn("Upd: payload invalid, too small")
		return nil, errors.New("payload invalid")
	}

	if len(buf) < idx+int(upd.DataLength) {
		logrus.Warn("Upd: failed to parse payload, data length exceeds buffer bounds")
		return nil, errors.New("payload invalid")
	}

	upd.Data = buf[idx : idx+int(upd.DataLength)]
	idx += int(upd.DataLength)

	return &upd, nil
}
