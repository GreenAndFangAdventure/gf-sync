// Package inbox writes the website's pending crafting orders into the
// data-only AddOn GreenFangToolkit_Inbox (decision D-018, format draft
// GF-006-C_inbox-format.md). It is the one place where GF Sync writes
// into the game folder, and it is off unless the build turns it on.
//
// The site sends one string ("GFIN1~<sum>~<sig>~H;...") signed with its
// private key. GF Sync checks the signature with a built-in public key
// and writes the string unchanged; it never builds or edits the content.
package inbox

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Format limits (draft §2, §3, D-018 item 4).
const (
	Version   = "GFIN1"
	MaxLen    = 65536 // bytes of the whole string
	MaxOrders = 200
	MaxSigLen = 200
)

var (
	ErrTooLarge  = errors.New("inbox: too large")
	ErrChars     = errors.New("inbox: characters outside the allowed set")
	ErrFormat    = errors.New("inbox: not a GFIN1 string")
	ErrSum       = errors.New("inbox: checksum does not match")
	ErrSignature = errors.New("inbox: signature does not verify")
	ErrNoKey     = errors.New("inbox: no public key")
)

// allowedChar is the whole alphabet of the format (draft §3c): letters,
// digits and * + . % _ ; = ~ -. No quote, backslash, slash, space or
// control character can ever reach Inbox.lua.
func allowedChar(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		return true
	}
	return strings.IndexByte("*+.%_;=~-", c) >= 0
}

// CheckChars reports whether s is non-empty, at most MaxLen bytes and only
// uses the format's alphabet.
func CheckChars(s string) error {
	if len(s) == 0 || len(s) > MaxLen {
		return ErrTooLarge
	}
	for i := 0; i < len(s); i++ {
		if !allowedChar(s[i]) {
			return fmt.Errorf("%w (byte %d)", ErrChars, i)
		}
	}
	return nil
}

// Sum is the checksum of draft §3d over body (from "H;" to the end):
// h = len mod M, then h = (h*31 + b) mod M for each byte, base 36.
func Sum(body string) string {
	const m = 16777213
	h := uint64(len(body)) % m
	for i := 0; i < len(body); i++ {
		h = (h*31 + uint64(body[i])) % m
	}
	return strconv.FormatUint(h, 36)
}

// SignedMessage is what the site signs: the string without the signature
// segment, "GFIN1~<sum>~<body>" (draft §7.3).
func SignedMessage(sum, body string) []byte {
	return []byte(Version + "~" + sum + "~" + body)
}

// Info is what Verify learned from the header.
type Info struct {
	At     int64 // H.at: when the site built the list (Unix seconds)
	Orders int   // number of O records (= H.n)
}

// Verify checks a whole inbox string: size and alphabet, version,
// checksum, the site's Ed25519 signature against any of keys, and the
// header's at and n. Only a string that passes is ever written.
func Verify(s string, keys []ed25519.PublicKey) (Info, error) {
	if len(keys) == 0 {
		return Info{}, ErrNoKey
	}
	if err := CheckChars(s); err != nil {
		return Info{}, err
	}
	parts := strings.SplitN(s, "~", 4)
	if len(parts) != 4 || parts[0] != Version {
		return Info{}, ErrFormat
	}
	sum, sigText, body := parts[1], parts[2], parts[3]
	if len(sum) == 0 || len(sum) > 8 || strings.Trim(sum, "0123456789abcdefghijklmnopqrstuvwxyz") != "" {
		return Info{}, ErrFormat
	}
	if len(sigText) == 0 || len(sigText) > MaxSigLen || strings.ContainsAny(sigText, "*+.%;=") {
		return Info{}, ErrFormat
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigText)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return Info{}, ErrFormat
	}
	ok := false
	msg := SignedMessage(sum, body)
	for _, k := range keys {
		if len(k) == ed25519.PublicKeySize && ed25519.Verify(k, msg, sig) {
			ok = true
			break
		}
	}
	if !ok {
		return Info{}, ErrSignature
	}
	// Signed by the site; the rest are sanity checks so that a site bug
	// cannot fill the game folder with something the AddOn would refuse.
	if Sum(body) != sum {
		return Info{}, ErrSum
	}
	return header(body)
}

// header reads H (the first record) and counts the O records.
func header(body string) (Info, error) {
	recs := strings.Split(body, "~")
	if !strings.HasPrefix(recs[0], "H;") {
		return Info{}, ErrFormat
	}
	kv := map[string]string{}
	for _, p := range strings.Split(recs[0][2:], ";") {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return Info{}, ErrFormat
		}
		if _, dup := kv[k]; dup {
			return Info{}, ErrFormat
		}
		kv[k] = v
	}
	at, err1 := strconv.ParseInt(kv["at"], 10, 64)
	n, err2 := strconv.Atoi(kv["n"])
	if err1 != nil || err2 != nil || at <= 0 || n < 0 || n > MaxOrders {
		return Info{}, ErrFormat
	}
	orders := 0
	for _, r := range recs[1:] {
		switch {
		case strings.HasPrefix(r, "O;"):
			orders++
		case strings.HasPrefix(r, "C;"):
		default:
			return Info{}, ErrFormat
		}
	}
	if orders != n {
		return Info{}, ErrFormat
	}
	return Info{At: at, Orders: orders}, nil
}
