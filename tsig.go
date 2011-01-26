package dns

// Implementation of TSIG: generation and validation
// RFC 2845 and RFC 4635
import (
	"io"
	"fmt"
	"strconv"
	"strings"
	"crypto/hmac"
	"encoding/hex"
)

// Need to lookup the actual codes
const (
	HmacMD5    = "HMAC-MD5.SIG-ALG.REG.INT"
	HmacSHA1   = "hmac-sha1"
	HmacSHA256 = "hmac-sha256"
)

type RR_TSIG struct {
	Hdr        RR_Header
	Algorithm  string "domain-name"
	TimeSigned uint64
	Fudge      uint16
	MACSize    uint16
	MAC        string "fixed-size"
	OrigId     uint16
	Error      uint16
	OtherLen   uint16
	OtherData  string "fixed-size"
}

func (rr *RR_TSIG) Header() *RR_Header {
	return &rr.Hdr
}

func (rr *RR_TSIG) String() string {
	// It has no official presentation format
	return rr.Hdr.String() +
		" " + rr.Algorithm +
		" " + tsigTimeToDate(rr.TimeSigned) +
		" " + strconv.Itoa(int(rr.Fudge)) +
		" " + strings.ToUpper(hex.EncodeToString([]byte(rr.MAC))) +
		" " + strconv.Itoa(int(rr.OrigId)) +
		" " + strconv.Itoa(int(rr.Error)) +
		" " + rr.OtherData
}

// The following values must be put in wireformat, so that the MAC can be calculated.
// RFC 2845, section 3.4.2. TSIG Variables.
type tsigWireFmt struct {
	// From RR_HEADER
	Name  string "domain-name"
	Class uint16
	Ttl   uint32
	// Rdata of the TSIG
	Algorithm  string "domain-name"
	TimeSigned uint64
	Fudge      uint16
	// MACSize, MAC and OrigId excluded
	Error     uint16
	OtherLen  uint16
	OtherData string "fixed-size"
}

// Return the RR with the TSIG AND include it in the message
// Generate the HMAC for msg. The TSIG RR is modified
// to include the MAC and MACSize. Note the the msg Id must
// be set, otherwise the MAC is not correct.
// The string 'secret' must be encoded in base64
func (rr *RR_TSIG) Generate(msg *Msg, secret string) bool {
	rawsecret := unpackBase64([]byte(secret))
	buf, ok := tsigToBuf(rr, msg)
	if !ok {
		return false
	}
	hmac := hmac.NewMD5([]byte(rawsecret))
	io.WriteString(hmac, string(buf))
	rr.MAC = string(hmac.Sum())
	rr.MACSize = uint16(len(rr.MAC))
	rr.OrigId = msg.MsgHdr.Id
	return true
}

// Verify a TSIG. The msg should be the complete message with
// the TSIG record still attached (as the last rr in the Additional
// section) TODO(mg)
// The secret is a base64 encoded string with a secret
func (rr *RR_TSIG) Verify(msg *Msg, secret string) bool {
	// copy the mesg, strip (and check) the tsig rr
	// perform the opposite of Generate() and then 
	// verify the mac
	rawsecret, err := packBase64([]byte(secret))
        if err != nil {
                return false
        }

	msg2 := msg // TODO deep copy TODO(mg)
	if len(msg2.Extra) < 1 {
		// nothing in additional
		return false
	}
	tsigrr := msg2.Extra[len(msg2.Extra)-1]
	if tsigrr.Header().Rrtype != TypeTSIG {
		// not a tsig RR
		return false
	}
	msg2.MsgHdr.Id = rr.OrigId
	msg2.Extra = msg2.Extra[:len(msg2.Extra)-1]     // Strip off the TSIG
	// TODO(mg)
	buf, ok := tsigToBuf(rr, msg2)
	if !ok {
		return false
	}
	h := hmac.NewMD5([]byte(rawsecret))
	io.WriteString(h, string(buf))
        return string(h.Sum()) == rr.MAC
}

func tsigToBuf(rr *RR_TSIG, msg *Msg) ([]byte, bool) {
	// Fill the struct and generate the wiredata
	buf := make([]byte, 4096) // TODO(mg) bufsize!
	tsig := new(tsigWireFmt)
	tsig.Name = rr.Header().Name
	tsig.Class = rr.Header().Class
	tsig.Ttl = rr.Header().Ttl
	tsig.Algorithm = rr.Algorithm
	tsig.TimeSigned = rr.TimeSigned
	tsig.Fudge = rr.Fudge
	tsig.Error = rr.Error
	tsig.OtherLen = rr.OtherLen
	tsig.OtherData = rr.OtherData
	n, ok1 := packStruct(tsig, buf, 0)
	if !ok1 {
		return nil, false
	}
	buf = buf[:n]
	msgbuf, ok := msg.Pack()
	if !ok {
		return nil, false
	}
        // First the pkg, then the tsig wire fmt
	buf = append(msgbuf, buf...)
        fmt.Printf("buf %v\n", buf)
	return buf, true
}
