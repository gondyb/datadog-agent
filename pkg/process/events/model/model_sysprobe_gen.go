package model

// Code generated by github.com/tinylib/msgp DO NOT EDIT.

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/tinylib/msgp/msgp"
)

// DecodeMsg implements msgp.Decodable
func (z *ProcessMonitoringEvent) DecodeMsg(dc *msgp.Reader) (err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, err = dc.ReadMapHeader()
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, err = dc.ReadMapKeyPtr()
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "ProcessCacheEntry":
			if dc.IsNil() {
				err = dc.ReadNil()
				if err != nil {
					err = msgp.WrapError(err, "ProcessCacheEntry")
					return
				}
				z.ProcessCacheEntry = nil
			} else {
				if z.ProcessCacheEntry == nil {
					z.ProcessCacheEntry = new(model.ProcessCacheEntry)
				}
				err = z.ProcessCacheEntry.DecodeMsg(dc)
				if err != nil {
					err = msgp.WrapError(err, "ProcessCacheEntry")
					return
				}
			}
		case "evt_type":
			z.EventType, err = dc.ReadString()
			if err != nil {
				err = msgp.WrapError(err, "EventType")
				return
			}
		case "collection_time":
			z.CollectionTime, err = dc.ReadTime()
			if err != nil {
				err = msgp.WrapError(err, "CollectionTime")
				return
			}
		default:
			err = dc.Skip()
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z *ProcessMonitoringEvent) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 3
	// write "ProcessCacheEntry"
	err = en.Append(0x83, 0xb1, 0x50, 0x72, 0x6f, 0x63, 0x65, 0x73, 0x73, 0x43, 0x61, 0x63, 0x68, 0x65, 0x45, 0x6e, 0x74, 0x72, 0x79)
	if err != nil {
		return
	}
	if z.ProcessCacheEntry == nil {
		err = en.WriteNil()
		if err != nil {
			return
		}
	} else {
		err = z.ProcessCacheEntry.EncodeMsg(en)
		if err != nil {
			err = msgp.WrapError(err, "ProcessCacheEntry")
			return
		}
	}
	// write "evt_type"
	err = en.Append(0xa8, 0x65, 0x76, 0x74, 0x5f, 0x74, 0x79, 0x70, 0x65)
	if err != nil {
		return
	}
	err = en.WriteString(z.EventType)
	if err != nil {
		err = msgp.WrapError(err, "EventType")
		return
	}
	// write "collection_time"
	err = en.Append(0xaf, 0x63, 0x6f, 0x6c, 0x6c, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x74, 0x69, 0x6d, 0x65)
	if err != nil {
		return
	}
	err = en.WriteTime(z.CollectionTime)
	if err != nil {
		err = msgp.WrapError(err, "CollectionTime")
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z *ProcessMonitoringEvent) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 3
	// string "ProcessCacheEntry"
	o = append(o, 0x83, 0xb1, 0x50, 0x72, 0x6f, 0x63, 0x65, 0x73, 0x73, 0x43, 0x61, 0x63, 0x68, 0x65, 0x45, 0x6e, 0x74, 0x72, 0x79)
	if z.ProcessCacheEntry == nil {
		o = msgp.AppendNil(o)
	} else {
		o, err = z.ProcessCacheEntry.MarshalMsg(o)
		if err != nil {
			err = msgp.WrapError(err, "ProcessCacheEntry")
			return
		}
	}
	// string "evt_type"
	o = append(o, 0xa8, 0x65, 0x76, 0x74, 0x5f, 0x74, 0x79, 0x70, 0x65)
	o = msgp.AppendString(o, z.EventType)
	// string "collection_time"
	o = append(o, 0xaf, 0x63, 0x6f, 0x6c, 0x6c, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x74, 0x69, 0x6d, 0x65)
	o = msgp.AppendTime(o, z.CollectionTime)
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *ProcessMonitoringEvent) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var zb0001 uint32
	zb0001, bts, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	for zb0001 > 0 {
		zb0001--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			err = msgp.WrapError(err)
			return
		}
		switch msgp.UnsafeString(field) {
		case "ProcessCacheEntry":
			if msgp.IsNil(bts) {
				bts, err = msgp.ReadNilBytes(bts)
				if err != nil {
					return
				}
				z.ProcessCacheEntry = nil
			} else {
				if z.ProcessCacheEntry == nil {
					z.ProcessCacheEntry = new(model.ProcessCacheEntry)
				}
				bts, err = z.ProcessCacheEntry.UnmarshalMsg(bts)
				if err != nil {
					err = msgp.WrapError(err, "ProcessCacheEntry")
					return
				}
			}
		case "evt_type":
			z.EventType, bts, err = msgp.ReadStringBytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "EventType")
				return
			}
		case "collection_time":
			z.CollectionTime, bts, err = msgp.ReadTimeBytes(bts)
			if err != nil {
				err = msgp.WrapError(err, "CollectionTime")
				return
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				err = msgp.WrapError(err)
				return
			}
		}
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z *ProcessMonitoringEvent) Msgsize() (s int) {
	s = 1 + 18
	if z.ProcessCacheEntry == nil {
		s += msgp.NilSize
	} else {
		s += z.ProcessCacheEntry.Msgsize()
	}
	s += 9 + msgp.StringPrefixSize + len(z.EventType) + 16 + msgp.TimeSize
	return
}
