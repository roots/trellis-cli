package engine

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // OpenPGP v4 fingerprints are defined as SHA-1
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// keyFingerprints returns the fingerprints of the primary keys in an
// OpenPGP public key (armored or binary), uppercase hex. It replaces
// `gpg --show-keys`, which isn't installed on every distribution before
// packages are.
func keyFingerprints(data []byte) ([]string, error) {
	if bytes.Contains(data, []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----")) {
		var err error
		if data, err = dearmor(data); err != nil {
			return nil, err
		}
	}
	var fprs []string
	for len(data) > 0 {
		tag, body, rest, err := nextPacket(data)
		if err != nil {
			return nil, err
		}
		data = rest
		if tag != 6 { // public key packet (primary key)
			continue
		}
		if len(body) == 0 {
			return nil, errors.New("empty public key packet")
		}
		switch body[0] {
		case 4:
			h := sha1.New() //nolint:gosec
			h.Write([]byte{0x99, byte(len(body) >> 8), byte(len(body))})
			h.Write(body)
			fprs = append(fprs, strings.ToUpper(hex.EncodeToString(h.Sum(nil))))
		case 6:
			h := sha256.New()
			var n [4]byte
			binary.BigEndian.PutUint32(n[:], uint32(len(body)))
			h.Write([]byte{0x9B})
			h.Write(n[:])
			h.Write(body)
			fprs = append(fprs, strings.ToUpper(hex.EncodeToString(h.Sum(nil))))
		default:
			return nil, fmt.Errorf("unsupported OpenPGP key version %d", body[0])
		}
	}
	if len(fprs) == 0 {
		return nil, errors.New("no OpenPGP public key found")
	}
	return fprs, nil
}

// nextPacket splits off one OpenPGP packet (RFC 9580 §4.2).
func nextPacket(data []byte) (tag int, body, rest []byte, err error) {
	b := data[0]
	if b&0x80 == 0 {
		return 0, nil, nil, errors.New("not an OpenPGP packet")
	}
	var n, hdr int
	if b&0x40 != 0 { // new format
		tag = int(b & 0x3f)
		if len(data) < 2 {
			return 0, nil, nil, errors.New("truncated packet")
		}
		switch o := int(data[1]); {
		case o < 192:
			n, hdr = o, 2
		case o < 224:
			if len(data) < 3 {
				return 0, nil, nil, errors.New("truncated packet")
			}
			n, hdr = (o-192)<<8+int(data[2])+192, 3
		case o == 255:
			if len(data) < 6 {
				return 0, nil, nil, errors.New("truncated packet")
			}
			n, hdr = int(binary.BigEndian.Uint32(data[2:6])), 6
		default:
			return 0, nil, nil, errors.New("partial body lengths aren't valid in keys")
		}
	} else { // old format
		tag = int(b>>2) & 0x0f
		switch b & 3 {
		case 0:
			if len(data) < 2 {
				return 0, nil, nil, errors.New("truncated packet")
			}
			n, hdr = int(data[1]), 2
		case 1:
			if len(data) < 3 {
				return 0, nil, nil, errors.New("truncated packet")
			}
			n, hdr = int(binary.BigEndian.Uint16(data[1:3])), 3
		case 2:
			if len(data) < 5 {
				return 0, nil, nil, errors.New("truncated packet")
			}
			n, hdr = int(binary.BigEndian.Uint32(data[1:5])), 5
		default:
			n, hdr = len(data)-1, 1
		}
	}
	if n < 0 || hdr+n > len(data) {
		return 0, nil, nil, errors.New("truncated packet")
	}
	return tag, data[hdr : hdr+n], data[hdr+n:], nil
}

// dearmor decodes ASCII-armored blocks (RFC 9580 §6.2). Key files may
// hold several blocks (nginx.org's has one per key); their packets are
// concatenated.
func dearmor(data []byte) ([]byte, error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	var out []byte
	var b64 strings.Builder
	in, body := false, false
	for _, l := range lines {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "-----BEGIN PGP PUBLIC KEY BLOCK"):
			in, body = true, false
			b64.Reset()
		case strings.HasPrefix(l, "-----END PGP"):
			if in {
				dec, err := base64.StdEncoding.DecodeString(b64.String())
				if err != nil {
					return nil, err
				}
				out = append(out, dec...)
			}
			in = false
		case !in:
		case !body:
			if l == "" {
				body = true // headers end at the first blank line
			} else if !strings.Contains(l, ": ") {
				body = true // no headers
				b64.WriteString(l)
			}
		case strings.HasPrefix(l, "=") && len(l) == 5:
			// CRC-24 checksum line
		default:
			b64.WriteString(l)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no armored key block")
	}
	return out, nil
}
