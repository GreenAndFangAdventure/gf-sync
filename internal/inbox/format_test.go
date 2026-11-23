package inbox_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"testing"

	"gfsync/internal/inbox"
	"gfsync/internal/mockapi"
)

// Made-up realm/guild (percent-encoded "測試"/"綠皮") and GUIDs.
const body = "H;realm=%E6%B8%AC%E8%A9%A6;guild=%E7%B6%A0%E7%9A%AE;at=1763900000;n=2" +
	"~C;cu=Player-0000-0000000B;t=1763899000" +
	"~O;id=w12;cr=1763890000;cf=*;rc=2660;it=2862;sk=164;q=5;m=2835*5;fe=500;st=open;sa=1763890000" +
	"~O;id=w13;cr=1763891000;cf=Player-0000-0000000C;rc=2661;q=1;fe=0;fq=0;st=cancel;sa=1763899000"

func testKeys() []ed25519.PublicKey { return []ed25519.PublicKey{inbox.TestPublicKey()} }

func TestTestKeyMatchesMock(t *testing.T) {
	pub := mockapi.InboxTestKey().Public().(ed25519.PublicKey)
	if hex.EncodeToString(pub) != inbox.TestPublicKeyHex {
		t.Fatal("inbox.TestPublicKeyHex is not the mock's key")
	}
	if len(inbox.SiteKeys) != 0 {
		t.Fatal("no production key exists yet; SiteKeys must stay empty until GearBot has one")
	}
	for _, k := range inbox.SiteKeys {
		if hex.EncodeToString(k) == inbox.TestPublicKeyHex {
			t.Fatal("the test key must never be a site key")
		}
	}
}

func TestVerifyGood(t *testing.T) {
	s := mockapi.SignInbox(mockapi.InboxTestKey(), body)
	info, err := inbox.Verify(s, testKeys())
	if err != nil || info.At != 1763900000 || info.Orders != 2 {
		t.Fatalf("Verify = %+v, %v", info, err)
	}
	// A second key (key change) still works.
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := inbox.Verify(s, []ed25519.PublicKey{other.Public().(ed25519.PublicKey), inbox.TestPublicKey()}); err != nil {
		t.Fatalf("old+new keys: %v", err)
	}
	// Empty list (header only) is valid.
	if info, err := inbox.Verify(mockapi.SignInbox(mockapi.InboxTestKey(), "H;realm=x;guild=y;at=5;n=0"), testKeys()); err != nil || info.Orders != 0 {
		t.Fatalf("empty list: %+v %v", info, err)
	}
}

func TestVerifyRefuses(t *testing.T) {
	key := mockapi.InboxTestKey()
	good := mockapi.SignInbox(key, body)
	_, otherKey, _ := ed25519.GenerateKey(rand.Reader)
	parts := strings.SplitN(good, "~", 4)
	cases := []struct {
		name string
		s    string
		want error
	}{
		{"signed with another key", mockapi.SignInbox(otherKey, body), inbox.ErrSignature},
		{"changed after signing", strings.Replace(good, "fe=500", "fe=900", 1), inbox.ErrSignature},
		{"order added after signing", good + "~O;id=w99;cr=1;cf=*;rc=1;q=1;fe=0;st=open;sa=1", inbox.ErrSignature},
		{"signature of another list", parts[0] + "~" + parts[1] + "~" + strings.SplitN(mockapi.SignInbox(key, "H;realm=x;guild=y;at=5;n=0"), "~", 4)[2] + "~" + parts[3], inbox.ErrSignature},
		{"no signature", parts[0] + "~" + parts[1] + "~~" + parts[3], inbox.ErrFormat},
		{"signature not base64url", parts[0] + "~" + parts[1] + "~abc=~" + parts[3], inbox.ErrFormat},
		{"other version", "GFIN2" + good[5:], inbox.ErrFormat},
		{"wrong checksum, signed", signWithSum(key, "zz", body), inbox.ErrSum},
		{"n does not match", mockapi.SignInbox(key, strings.Replace(body, "n=2", "n=3", 1)), inbox.ErrFormat},
		{"more than 200 orders", mockapi.SignInbox(key, manyOrders(201)), inbox.ErrFormat},
		{"duplicate header key", mockapi.SignInbox(key, "H;at=5;at=6;n=0"), inbox.ErrFormat},
		{"no header", mockapi.SignInbox(key, "C;cu=x;t=1"), inbox.ErrFormat},
		{"quote", mockapi.SignInbox(key, body+`"`), inbox.ErrChars},
		{"backslash path", mockapi.SignInbox(key, `H;realm=..\..\WTF;guild=y;at=5;n=0`), inbox.ErrChars},
		{"slash path", mockapi.SignInbox(key, `H;realm=../../WTF;guild=y;at=5;n=0`), inbox.ErrChars},
		{"newline (Lua code)", mockapi.SignInbox(key, "H;at=5;n=0\nos.execute()"), inbox.ErrChars},
		{"space", mockapi.SignInbox(key, "H;realm=a b;at=5;n=0"), inbox.ErrChars},
		{"not ASCII", mockapi.SignInbox(key, "H;realm=測試;at=5;n=0"), inbox.ErrChars},
		{"too large", mockapi.SignInbox(key, "H;realm="+strings.Repeat("x", inbox.MaxLen)+";at=5;n=0"), inbox.ErrTooLarge},
		{"empty", "", inbox.ErrTooLarge},
	}
	for _, c := range cases {
		if _, err := inbox.Verify(c.s, testKeys()); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	if _, err := inbox.Verify(good, nil); !errors.Is(err, inbox.ErrNoKey) {
		t.Errorf("no keys: %v", err)
	}
}

func signWithSum(key ed25519.PrivateKey, sum, body string) string {
	sig := ed25519.Sign(key, inbox.SignedMessage(sum, body))
	return inbox.Version + "~" + sum + "~" + base64.RawURLEncoding.EncodeToString(sig) + "~" + body
}

func manyOrders(n int) string {
	var b strings.Builder
	b.WriteString("H;realm=x;guild=y;at=5;n=")
	b.WriteString(strconv.Itoa(n))
	b.WriteString("~C;cu=Player-0000-0000000B;t=1")
	for i := 0; i < n; i++ {
		b.WriteString("~O;id=w" + strconv.Itoa(i) + ";cr=1;cf=*;rc=1;q=1;fe=0;st=open;sa=1")
	}
	return b.String()
}
