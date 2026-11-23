package inbox

import (
	"crypto/ed25519"
	"encoding/hex"
)

// SiteKeys are the website's public keys for the inbox signature. Empty
// until the site has a production key (GearBot); with no key the feature
// stays off. During a key change both keys are listed for a while.
var SiteKeys = []ed25519.PublicKey{}

// TestPublicKeyHex is the public half of the gfsync-mock test key
// (mockapi.InboxTestKey). It is NOT a production key: GF Sync accepts it
// only when talking to a loopback test server.
const TestPublicKeyHex = "cc00832bd0c735acdba6d4f61b3daae40ea8bba1c291bcdf5e33dd50573b65ba"

// TestPublicKey decodes TestPublicKeyHex.
func TestPublicKey() ed25519.PublicKey {
	b, err := hex.DecodeString(TestPublicKeyHex)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil
	}
	return ed25519.PublicKey(b)
}
