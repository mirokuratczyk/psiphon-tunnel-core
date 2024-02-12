// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tls

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"math/big"
	math_rand "math/rand"

	"golang.org/x/crypto/cryptobyte"
)

// [Psiphon]
var obfuscateSessionTickets = true

// A sessionState is a resumable session.
type SessionState = tls.SessionState
type sessionState struct {
	// Encoded as a SessionState (in the language of RFC 8446, Section 3).
	//
	//   enum { server(1), client(2) } SessionStateType;
	//
	//   opaque Certificate<1..2^24-1>;
	//
	//   Certificate CertificateChain<0..2^24-1>;
	//
	//   opaque Extra<0..2^24-1>;
	//
	//   struct {
	//       uint16 version;
	//       SessionStateType type;
	//       uint16 cipher_suite;
	//       uint64 created_at;
	//       opaque secret<1..2^8-1>;
	//       Extra extra<0..2^24-1>;
	//       uint8 ext_master_secret = { 0, 1 };
	//       uint8 early_data = { 0, 1 };
	//       CertificateEntry certificate_list<0..2^24-1>;
	//       CertificateChain verified_chains<0..2^24-1>; /* excluding leaf */
	//       select (SessionState.early_data) {
	//           case 0: Empty;
	//           case 1: opaque alpn<1..2^8-1>;
	//       };
	//       select (SessionState.type) {
	//           case server: Empty;
	//           case client: struct {
	//               select (SessionState.version) {
	//                   case VersionTLS10..VersionTLS12: Empty;
	//                   case VersionTLS13: struct {
	//                       uint64 use_by;
	//                       uint32 age_add;
	//                   };
	//               };
	//           };
	//       };
	//   } SessionState;
	//

	// Extra is ignored by crypto/tls, but is encoded by [SessionState.Bytes]
	// and parsed by [ParseSessionState].
	//
	// This allows [Config.UnwrapSession]/[Config.WrapSession] and
	// [ClientSessionCache] implementations to store and retrieve additional
	// data alongside this session.
	//
	// To allow different layers in a protocol stack to share this field,
	// applications must only append to it, not replace it, and must use entries
	// that can be recognized even if out of order (for example, by starting
	// with a id and version prefix).
	Extra [][]byte

	// EarlyData indicates whether the ticket can be used for 0-RTT in a QUIC
	// connection. The application may set this to false if it is true to
	// decline to offer 0-RTT even if supported.
	EarlyData bool

	version     uint16
	isClient    bool
	cipherSuite uint16
	// createdAt is the generation time of the secret on the sever (which for
	// TLS 1.0–1.2 might be earlier than the current session) and the time at
	// which the ticket was received on the client.
	createdAt         uint64 // seconds since UNIX epoch
	secret            []byte // master secret for TLS 1.2, or the PSK for TLS 1.3
	extMasterSecret   bool
	peerCertificates  []*x509.Certificate
	activeCertHandles []*activeCert
	ocspResponse      []byte
	scts              [][]byte
	verifiedChains    [][]*x509.Certificate
	alpnProtocol      string // only set if EarlyData is true

	// Client-side TLS 1.3-only fields.
	useBy  uint64 // seconds since UNIX epoch
	ageAdd uint32
}

// Bytes encodes the session, including any private fields, so that it can be
// parsed by [ParseSessionState]. The encoding contains secret values critical
// to the security of future and possibly past sessions.
//
// The specific encoding should be considered opaque and may change incompatibly
// between Go versions.
func (s *sessionState) Bytes() ([]byte, error) {
	var b cryptobyte.Builder
	b.AddUint16(s.version)
	if s.isClient {
		b.AddUint8(2) // client
	} else {
		b.AddUint8(1) // server
	}
	b.AddUint16(s.cipherSuite)
	addUint64(&b, s.createdAt)
	b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) {
		b.AddBytes(s.secret)
	})
	b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) {
		for _, extra := range s.Extra {
			b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) {
				b.AddBytes(extra)
			})
		}
	})
	if s.extMasterSecret {
		b.AddUint8(1)
	} else {
		b.AddUint8(0)
	}
	if s.EarlyData {
		b.AddUint8(1)
	} else {
		b.AddUint8(0)
	}
	marshalCertificate(&b, Certificate{
		Certificate:                 certificatesToBytesSlice(s.peerCertificates),
		OCSPStaple:                  s.ocspResponse,
		SignedCertificateTimestamps: s.scts,
	})
	b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) {
		for _, chain := range s.verifiedChains {
			b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) {
				// We elide the first certificate because it's always the leaf.
				if len(chain) == 0 {
					b.SetError(errors.New("tls: internal error: empty verified chain"))
					return
				}
				for _, cert := range chain[1:] {
					b.AddUint24LengthPrefixed(func(b *cryptobyte.Builder) {
						b.AddBytes(cert.Raw)
					})
				}
			})
		}
	})
	if s.EarlyData {
		b.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) {
			b.AddBytes([]byte(s.alpnProtocol))
		})
	}
	if s.isClient {
		if s.version >= VersionTLS13 {
			addUint64(&b, s.useBy)
			b.AddUint32(s.ageAdd)
		}
	}

	// [Psiphon]
	bytes, err := b.Bytes()
	if err != nil {
		return nil, err
	}

	// [Psiphon]
	// Pad golang TLS session ticket to a more typical size.
	if obfuscateSessionTickets {
		paddedSizes := []int{160, 176, 192, 208, 218, 224, 240, 255}
		initialSize := 120
		randomInt, err := rand.Int(rand.Reader, big.NewInt(int64(len(paddedSizes))))
		index := 0
		if err == nil {
			index = int(randomInt.Int64())
		} else {
			index = math_rand.Intn(len(paddedSizes))
		}
		paddingSize := paddedSizes[index] - initialSize

		ret := make([]byte, len(bytes)+paddingSize)
		copy(ret, bytes)
		return ret, nil
	}

	return bytes, nil
}

func certificatesToBytesSlice(certs []*x509.Certificate) [][]byte {
	s := make([][]byte, 0, len(certs))
	for _, c := range certs {
		s = append(s, c.Raw)
	}
	return s
}

// ParseSessionState parses a [SessionState] encoded by [SessionState.Bytes].
func ParseSessionState(data []byte) (*sessionState, error) {
	ss := &sessionState{}
	s := cryptobyte.String(data)
	var typ, extMasterSecret, earlyData uint8
	var cert Certificate
	var extra cryptobyte.String
	if !s.ReadUint16(&ss.version) ||
		!s.ReadUint8(&typ) ||
		(typ != 1 && typ != 2) ||
		!s.ReadUint16(&ss.cipherSuite) ||
		!readUint64(&s, &ss.createdAt) ||
		!readUint8LengthPrefixed(&s, &ss.secret) ||
		!s.ReadUint24LengthPrefixed(&extra) ||
		!s.ReadUint8(&extMasterSecret) ||
		!s.ReadUint8(&earlyData) ||
		len(ss.secret) == 0 ||
		!unmarshalCertificate(&s, &cert) {
		return nil, errors.New("tls: invalid session encoding")
	}
	for !extra.Empty() {
		var e []byte
		if !readUint24LengthPrefixed(&extra, &e) {
			return nil, errors.New("tls: invalid session encoding")
		}
		ss.Extra = append(ss.Extra, e)
	}
	switch extMasterSecret {
	case 0:
		ss.extMasterSecret = false
	case 1:
		ss.extMasterSecret = true
	default:
		return nil, errors.New("tls: invalid session encoding")
	}
	switch earlyData {
	case 0:
		ss.EarlyData = false
	case 1:
		ss.EarlyData = true
	default:
		return nil, errors.New("tls: invalid session encoding")
	}
	for _, cert := range cert.Certificate {
		c, err := globalCertCache.newCert(cert)
		if err != nil {
			return nil, err
		}
		ss.activeCertHandles = append(ss.activeCertHandles, c)
		ss.peerCertificates = append(ss.peerCertificates, c.cert)
	}
	ss.ocspResponse = cert.OCSPStaple
	ss.scts = cert.SignedCertificateTimestamps
	var chainList cryptobyte.String
	if !s.ReadUint24LengthPrefixed(&chainList) {
		return nil, errors.New("tls: invalid session encoding")
	}
	for !chainList.Empty() {
		var certList cryptobyte.String
		if !chainList.ReadUint24LengthPrefixed(&certList) {
			return nil, errors.New("tls: invalid session encoding")
		}
		var chain []*x509.Certificate
		if len(ss.peerCertificates) == 0 {
			return nil, errors.New("tls: invalid session encoding")
		}
		chain = append(chain, ss.peerCertificates[0])
		for !certList.Empty() {
			var cert []byte
			if !readUint24LengthPrefixed(&certList, &cert) {
				return nil, errors.New("tls: invalid session encoding")
			}
			c, err := globalCertCache.newCert(cert)
			if err != nil {
				return nil, err
			}
			ss.activeCertHandles = append(ss.activeCertHandles, c)
			chain = append(chain, c.cert)
		}
		ss.verifiedChains = append(ss.verifiedChains, chain)
	}
	if ss.EarlyData {
		var alpn []byte
		if !readUint8LengthPrefixed(&s, &alpn) {
			return nil, errors.New("tls: invalid session encoding")
		}
		ss.alpnProtocol = string(alpn)
	}
	if isClient := typ == 2; !isClient {
		// [Psiphon]
		// Ignore padding for obfuscated session tickets.
		if !s.Empty() && !allZeros(s) {
			return nil, errors.New("tls: invalid session encoding")
		}
		return ss, nil
	}
	ss.isClient = true
	if len(ss.peerCertificates) == 0 {
		return nil, errors.New("tls: no server certificates in client session")
	}
	if ss.version < VersionTLS13 {
		if !s.Empty() {
			return nil, errors.New("tls: invalid session encoding")
		}
		return ss, nil
	}
	if !s.ReadUint64(&ss.useBy) || !s.ReadUint32(&ss.ageAdd) || !s.Empty() {
		return nil, errors.New("tls: invalid session encoding")
	}
	return ss, nil
}

// sessionState returns a partially filled-out [SessionState] with information
// from the current connection.
func (c *Conn) sessionState() (*sessionState, error) {
	return &sessionState{
		version:           c.vers,
		cipherSuite:       c.cipherSuite,
		createdAt:         uint64(c.config.time().Unix()),
		alpnProtocol:      c.clientProtocol,
		peerCertificates:  c.peerCertificates,
		activeCertHandles: c.activeCertHandles,
		ocspResponse:      c.ocspResponse,
		scts:              c.scts,
		isClient:          c.isClient,
		extMasterSecret:   c.extMasterSecret,
		verifiedChains:    c.verifiedChains,
	}, nil
}

// EncryptTicket encrypts a ticket with the Config's configured (or default)
// session ticket keys. It can be used as a [Config.WrapSession] implementation.
func (c *config) EncryptTicket(cs ConnectionState, ss *sessionState) ([]byte, error) {
	ticketKeys := c.ticketKeys(nil)
	stateBytes, err := ss.Bytes()
	if err != nil {
		return nil, err
	}
	return c.encryptTicket(stateBytes, ticketKeys)
}

func (c *config) encryptTicket(state []byte, ticketKeys []ticketKey) ([]byte, error) {
	if len(ticketKeys) == 0 {
		return nil, errors.New("tls: internal error: session ticket keys unavailable")
	}

	encrypted := make([]byte, aes.BlockSize+len(state)+sha256.Size)
	iv := encrypted[:aes.BlockSize]
	ciphertext := encrypted[aes.BlockSize : len(encrypted)-sha256.Size]
	authenticated := encrypted[:len(encrypted)-sha256.Size]
	macBytes := encrypted[len(encrypted)-sha256.Size:]

	if _, err := io.ReadFull(c.rand(), iv); err != nil {
		return nil, err
	}
	key := ticketKeys[0]
	block, err := aes.NewCipher(key.aesKey[:])
	if err != nil {
		return nil, errors.New("tls: failed to create cipher while encrypting ticket: " + err.Error())
	}
	cipher.NewCTR(block, iv).XORKeyStream(ciphertext, state)

	mac := hmac.New(sha256.New, key.hmacKey[:])
	mac.Write(authenticated)
	mac.Sum(macBytes[:0])

	return encrypted, nil
}

// DecryptTicket decrypts a ticket encrypted by [Config.EncryptTicket]. It can
// be used as a [Config.UnwrapSession] implementation.
//
// If the ticket can't be decrypted or parsed, DecryptTicket returns (nil, nil).
func (c *config) DecryptTicket(identity []byte, cs ConnectionState) (*sessionState, error) {
	ticketKeys := c.ticketKeys(nil)
	stateBytes := c.decryptTicket(identity, ticketKeys)
	if stateBytes == nil {
		return nil, nil
	}
	s, err := ParseSessionState(stateBytes)
	if err != nil {
		return nil, nil // drop unparsable tickets on the floor
	}
	return s, nil
}

func (c *config) decryptTicket(encrypted []byte, ticketKeys []ticketKey) []byte {
	if len(encrypted) < aes.BlockSize+sha256.Size {
		return nil
	}

	iv := encrypted[:aes.BlockSize]
	ciphertext := encrypted[aes.BlockSize : len(encrypted)-sha256.Size]
	authenticated := encrypted[:len(encrypted)-sha256.Size]
	macBytes := encrypted[len(encrypted)-sha256.Size:]

	for _, key := range ticketKeys {
		mac := hmac.New(sha256.New, key.hmacKey[:])
		mac.Write(authenticated)
		expected := mac.Sum(nil)

		if subtle.ConstantTimeCompare(macBytes, expected) != 1 {
			continue
		}

		block, err := aes.NewCipher(key.aesKey[:])
		if err != nil {
			return nil
		}
		plaintext := make([]byte, len(ciphertext))
		cipher.NewCTR(block, iv).XORKeyStream(plaintext, ciphertext)

		return plaintext
	}

	return nil
}

type ClientSessionState = tls.ClientSessionState

// ClientSessionState contains the state needed by a client to
// resume a previous TLS session.
type clientSessionState struct {
	ticket  []byte
	session *sessionState
}

// ResumptionState returns the session ticket sent by the server (also known as
// the session's identity) and the state necessary to resume this session.
//
// It can be called by [ClientSessionCache.Put] to serialize (with
// [SessionState.Bytes]) and store the session.
func (cs *clientSessionState) ResumptionState() (ticket []byte, state *sessionState, err error) {
	return cs.ticket, cs.session, nil
}

// NewResumptionState returns a state value that can be returned by
// [ClientSessionCache.Get] to resume a previous session.
//
// state needs to be returned by [ParseSessionState], and the ticket and session
// state must have been returned by [ClientSessionState.ResumptionState].
func NewResumptionState(ticket []byte, state *sessionState) (*ClientSessionState, error) {
	cs := &clientSessionState{
		ticket: ticket, session: state,
	}
	return toClientSessionState(cs), nil
}

// [Psiphon]
type ObfuscatedClientSessionState struct {
	SessionTicket      []uint8
	Vers               uint16
	CipherSuite        uint16
	MasterSecret       []byte
	ServerCertificates []*x509.Certificate
	VerifiedChains     [][]*x509.Certificate
	UseEMS             bool
}

var obfuscatedSessionTicketCipherSuite = TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256

// [Psiphon]
// NewObfuscatedClientSessionState produces obfuscated session tickets.
//
// # Obfuscated Session Tickets
//
// Obfuscated session tickets is a network traffic obfuscation protocol that appears
// to be valid TLS using session tickets. The client actually generates the session
// ticket and encrypts it with a shared secret, enabling a TLS session that entirely
// skips the most fingerprintable aspects of TLS.
// The scheme is described here:
// https://lists.torproject.org/pipermail/tor-dev/2016-September/011354.html
//
// Circumvention notes:
//   - TLS session ticket implementations are widespread:
//     https://istlsfastyet.com/#cdn-paas.
//   - An adversary cannot easily block session ticket capability, as this requires
//     a downgrade attack against TLS.
//   - Anti-probing defence is provided, as the adversary must use the correct obfuscation
//     shared secret to form valid obfuscation session ticket; otherwise server offers
//     standard session tickets.
//   - Limitation: an adversary with the obfuscation shared secret can decrypt the session
//     ticket and observe the plaintext traffic. It's assumed that the adversary will not
//     learn the obfuscated shared secret without also learning the address of the TLS
//     server and blocking it anyway; it's also assumed that the TLS payload is not
//     plaintext but is protected with some other security layer (e.g., SSH).
//
// Implementation notes:
//   - The TLS ClientHello includes an SNI field, even when using session tickets, so
//     the client should populate the ServerName.
//   - Server should set its SetSessionTicketKeys with first a standard key, followed by
//     the obfuscation shared secret.
//   - Since the client creates the session ticket, it selects parameters that were not
//     negotiated with the server, such as the cipher suite. It's implicitly assumed that
//     the server can support the selected parameters.
//   - Obfuscated session tickets are not supported for TLS 1.3 _clients_, which use a
//     distinct scheme. Obfuscated session ticket support in this package is intended to
//     support TLS 1.2 clients.
func NewObfuscatedClientSessionState(sharedSecret [32]byte) (*ObfuscatedClientSessionState, error) {

	// Create a session ticket that wasn't actually issued by the server.
	vers := uint16(VersionTLS12)
	cipherSuite := obfuscatedSessionTicketCipherSuite
	masterSecret := make([]byte, masterSecretLength)
	_, err := rand.Read(masterSecret)
	if err != nil {
		return nil, err
	}

	serverState := &sessionState{
		version:          vers,
		cipherSuite:      cipherSuite,
		secret:           masterSecret,
		peerCertificates: nil,
	}

	config := &config{}
	sessionTicketKeys := []ticketKey{config.ticketKeyFromBytes(sharedSecret)}

	ssBytes, err := serverState.Bytes()
	if err != nil {
		return nil, err
	}
	sessionTicket, err := config.encryptTicket(ssBytes, sessionTicketKeys)
	if err != nil {
		return nil, err
	}

	// ObfuscatedClientSessionState fields are used to construct
	// ClientSessionState objects for use in ClientSessionCaches. The client will
	// use this cache to pretend it got that session ticket from the server.
	clientState := &ObfuscatedClientSessionState{
		SessionTicket: sessionTicket,
		Vers:          vers,
		CipherSuite:   cipherSuite,
		MasterSecret:  masterSecret,
	}

	return clientState, nil
}

func ContainsObfuscatedSessionTicketCipherSuite(cipherSuites []uint16) bool {
	for _, cipherSuite := range cipherSuites {
		if cipherSuite == obfuscatedSessionTicketCipherSuite {
			return true
		}
	}
	return false
}

// allZeros returns true if remaining bytes are all zero.
func allZeros(s cryptobyte.String) bool {
	b := []byte(s)
	for _, v := range b {
		if v != 0x0 {
			return false
		}
	}
	return true
}
