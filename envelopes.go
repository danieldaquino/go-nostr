package nostr

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	"github.com/mailru/easyjson"
	jwriter "github.com/mailru/easyjson/jwriter"
	"github.com/tidwall/gjson"
)

var (
	labelEvent  = []byte("EVENT")
	labelReq    = []byte("REQ")
	labelCount  = []byte("COUNT")
	labelNotice = []byte("NOTICE")
	labelEose   = []byte("EOSE")
	labelOk     = []byte("OK")
	labelAuth   = []byte("AUTH")
	labelClosed = []byte("CLOSED")
	labelClose  = []byte("CLOSE")

	UnknownLabel = errors.New("unknown envelope label")
)

// ParseMessage parses a message into an Envelope.
func ParseMessage(message []byte) Envelope {
	firstComma := bytes.Index(message, []byte{','})
	if firstComma == -1 {
		return nil
	}
	label := message[0:firstComma]

	var v Envelope
	switch {
	case bytes.Contains(label, labelEvent):
		v = &EventEnvelope{}
	case bytes.Contains(label, labelReq):
		v = &ReqEnvelope{}
	case bytes.Contains(label, labelCount):
		v = &CountEnvelope{}
	case bytes.Contains(label, labelNotice):
		x := NoticeEnvelope("")
		v = &x
	case bytes.Contains(label, labelEose):
		x := EOSEEnvelope("")
		v = &x
	case bytes.Contains(label, labelOk):
		v = &OKEnvelope{}
	case bytes.Contains(label, labelAuth):
		v = &AuthEnvelope{}
	case bytes.Contains(label, labelClosed):
		v = &ClosedEnvelope{}
	case bytes.Contains(label, labelClose):
		x := CloseEnvelope("")
		v = &x
	default:
		return nil
	}

	if err := v.UnmarshalJSON(message); err != nil {
		return nil
	}
	return v
}

// Envelope is the interface for all nostr message envelopes.
type Envelope interface {
	Label() string
	UnmarshalJSON([]byte) error
	MarshalJSON() ([]byte, error)
	String() string
}

var (
	_ Envelope = (*EventEnvelope)(nil)
	_ Envelope = (*ReqEnvelope)(nil)
	_ Envelope = (*CountEnvelope)(nil)
	_ Envelope = (*NoticeEnvelope)(nil)
	_ Envelope = (*EOSEEnvelope)(nil)
	_ Envelope = (*CloseEnvelope)(nil)
	_ Envelope = (*OKEnvelope)(nil)
	_ Envelope = (*AuthEnvelope)(nil)
)

// EventEnvelope represents an EVENT message.
type EventEnvelope struct {
	SubscriptionID *string
	Event
}

func (_ EventEnvelope) Label() string { return "EVENT" }

func (v *EventEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()
	switch len(arr) {
	case 2:
		return easyjson.Unmarshal([]byte(arr[1].Raw), &v.Event)
	case 3:
		v.SubscriptionID = &arr[1].Str
		return easyjson.Unmarshal([]byte(arr[2].Raw), &v.Event)
	default:
		return fmt.Errorf("failed to decode EVENT envelope")
	}
}

func (v EventEnvelope) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{NoEscapeHTML: true}
	w.RawString(`["EVENT",`)
	if v.SubscriptionID != nil {
		w.RawString(`"`)
		w.RawString(*v.SubscriptionID)
		w.RawString(`",`)
	}
	v.Event.MarshalEasyJSON(&w)
	w.RawString(`]`)
	return w.BuildBytes()
}

// ReqEnvelope represents a REQ message.
type ReqEnvelope struct {
	SubscriptionID string
	Filters
}

func (_ ReqEnvelope) Label() string { return "REQ" }

func (v *ReqEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()
	if len(arr) < 3 {
		return fmt.Errorf("failed to decode REQ envelope: missing filters")
	}
	v.SubscriptionID = arr[1].Str
	v.Filters = make(Filters, len(arr)-2)
	f := 0
	for i := 2; i < len(arr); i++ {
		if err := easyjson.Unmarshal([]byte(arr[i].Raw), &v.Filters[f]); err != nil {
			return fmt.Errorf("%w -- on filter %d", err, f)
		}
		f++
	}

	return nil
}

func (v ReqEnvelope) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{NoEscapeHTML: true}
	w.RawString(`["REQ","`)
	w.RawString(v.SubscriptionID)
	w.RawString(`"`)
	for _, filter := range v.Filters {
		w.RawString(`,`)
		filter.MarshalEasyJSON(&w)
	}
	w.RawString(`]`)
	return w.BuildBytes()
}

// CountEnvelope represents a COUNT message.
type CountEnvelope struct {
	SubscriptionID string
	Filters
	Count       *int64
	HyperLogLog []byte
}

func (_ CountEnvelope) Label() string { return "COUNT" }
func (c CountEnvelope) String() string {
	v, _ := json.Marshal(c)
	return string(v)
}

func (v *CountEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()
	if len(arr) < 3 {
		return fmt.Errorf("failed to decode COUNT envelope: missing filters")
	}
	v.SubscriptionID = arr[1].Str

	var countResult struct {
		Count *int64 `json:"count"`
		HLL   string `json:"hll"`
	}
	if err := json.Unmarshal([]byte(arr[2].Raw), &countResult); err == nil && countResult.Count != nil {
		v.Count = countResult.Count
		if len(countResult.HLL) == 512 {
			v.HyperLogLog, err = hex.DecodeString(countResult.HLL)
			if err != nil {
				return fmt.Errorf("invalid \"hll\" value in COUNT message: %w", err)
			}
		}
		return nil
	}

	v.Filters = make(Filters, len(arr)-2)
	f := 0
	for i := 2; i < len(arr); i++ {
		item := []byte(arr[i].Raw)

		if err := easyjson.Unmarshal(item, &v.Filters[f]); err != nil {
			return fmt.Errorf("%w -- on filter %d", err, f)
		}

		f++
	}

	return nil
}

func (v CountEnvelope) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{NoEscapeHTML: true}
	w.RawString(`["COUNT","`)
	w.RawString(v.SubscriptionID)
	w.RawString(`"`)
	if v.Count != nil {
		w.RawString(`,{"count":`)
		w.RawString(strconv.FormatInt(*v.Count, 10))
		if v.HyperLogLog != nil {
			w.RawString(`,"hll":"`)
			hllHex := make([]byte, 512)
			hex.Encode(hllHex, v.HyperLogLog)
			w.Buffer.AppendBytes(hllHex)
			w.RawString(`"`)
		}
		w.RawString(`}`)
	} else {
		for _, filter := range v.Filters {
			w.RawString(`,`)
			filter.MarshalEasyJSON(&w)
		}
	}
	w.RawString(`]`)
	return w.BuildBytes()
}

// NoticeEnvelope represents a NOTICE message.
type NoticeEnvelope string

func (_ NoticeEnvelope) Label() string { return "NOTICE" }
func (n NoticeEnvelope) String() string {
	v, _ := json.Marshal(n)
	return string(v)
}

func (v *NoticeEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()
	if len(arr) < 2 {
		return fmt.Errorf("failed to decode NOTICE envelope")
	}
	*v = NoticeEnvelope(arr[1].Str)
	return nil
}

func (v NoticeEnvelope) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{NoEscapeHTML: true}
	w.RawString(`["NOTICE",`)
	w.Raw(json.Marshal(string(v)))
	w.RawString(`]`)
	return w.BuildBytes()
}

// EOSEEnvelope represents an EOSE (End of Stored Events) message.
type EOSEEnvelope string

func (_ EOSEEnvelope) Label() string { return "EOSE" }
func (e EOSEEnvelope) String() string {
	v, _ := json.Marshal(e)
	return string(v)
}

func (v *EOSEEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()
	if len(arr) < 2 {
		return fmt.Errorf("failed to decode EOSE envelope")
	}
	*v = EOSEEnvelope(arr[1].Str)
	return nil
}

func (v EOSEEnvelope) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{NoEscapeHTML: true}
	w.RawString(`["EOSE",`)
	w.Raw(json.Marshal(string(v)))
	w.RawString(`]`)
	return w.BuildBytes()
}

// CloseEnvelope represents a CLOSE message.
type CloseEnvelope string

func (_ CloseEnvelope) Label() string { return "CLOSE" }
func (c CloseEnvelope) String() string {
	v, _ := json.Marshal(c)
	return string(v)
}

func (v *CloseEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()
	switch len(arr) {
	case 2:
		*v = CloseEnvelope(arr[1].Str)
		return nil
	default:
		return fmt.Errorf("failed to decode CLOSE envelope")
	}
}

func (v CloseEnvelope) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{NoEscapeHTML: true}
	w.RawString(`["CLOSE",`)
	w.Raw(json.Marshal(string(v)))
	w.RawString(`]`)
	return w.BuildBytes()
}

// ClosedEnvelope represents a CLOSED message.
type ClosedEnvelope struct {
	SubscriptionID string
	Reason         string
}

func (_ ClosedEnvelope) Label() string { return "CLOSED" }
func (c ClosedEnvelope) String() string {
	v, _ := json.Marshal(c)
	return string(v)
}

func (v *ClosedEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()
	switch len(arr) {
	case 3:
		*v = ClosedEnvelope{arr[1].Str, arr[2].Str}
		return nil
	default:
		return fmt.Errorf("failed to decode CLOSED envelope")
	}
}

func (v ClosedEnvelope) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{NoEscapeHTML: true}
	w.RawString(`["CLOSED",`)
	w.Raw(json.Marshal(string(v.SubscriptionID)))
	w.RawString(`,`)
	w.Raw(json.Marshal(v.Reason))
	w.RawString(`]`)
	return w.BuildBytes()
}

// OKEnvelope represents an OK message.
type OKEnvelope struct {
	EventID string
	OK      bool
	Reason  string
}

func (_ OKEnvelope) Label() string { return "OK" }
func (o OKEnvelope) String() string {
	v, _ := json.Marshal(o)
	return string(v)
}

func (v *OKEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()
	if len(arr) < 4 {
		return fmt.Errorf("failed to decode OK envelope: missing fields")
	}
	v.EventID = arr[1].Str
	v.OK = arr[2].Raw == "true"
	v.Reason = arr[3].Str

	return nil
}

func (v OKEnvelope) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{NoEscapeHTML: true}
	w.RawString(`["OK","`)
	w.RawString(v.EventID)
	w.RawString(`",`)
	ok := "false"
	if v.OK {
		ok = "true"
	}
	w.RawString(ok)
	w.RawString(`,`)
	w.Raw(json.Marshal(v.Reason))
	w.RawString(`]`)
	return w.BuildBytes()
}

// AuthEnvelope represents an AUTH message.
type AuthEnvelope struct {
	Challenge *string
	Event     Event
}

func (_ AuthEnvelope) Label() string { return "AUTH" }
func (a AuthEnvelope) String() string {
	v, _ := json.Marshal(a)
	return string(v)
}

func (v *AuthEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()
	if len(arr) < 2 {
		return fmt.Errorf("failed to decode Auth envelope: missing fields")
	}
	if arr[1].IsObject() {
		return easyjson.Unmarshal([]byte(arr[1].Raw), &v.Event)
	} else {
		v.Challenge = &arr[1].Str
	}
	return nil
}

func (v AuthEnvelope) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{NoEscapeHTML: true}
	w.RawString(`["AUTH",`)
	if v.Challenge != nil {
		w.Raw(json.Marshal(*v.Challenge))
	} else {
		v.Event.MarshalEasyJSON(&w)
	}
	w.RawString(`]`)
	return w.BuildBytes()
}
