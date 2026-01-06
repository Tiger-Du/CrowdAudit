package authmw

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ctxKey string

const (
	CtxUser ctxKey = "auth.user"
)

type User struct {
	Subject string
	Email   string
	Issuer  string
	Raw     map[string]any
}

type Config struct {
	// Allow multiple issuers for easy Auth0<->Cognito migrations, or staging/prod simultaneously.
	Providers []ProviderConfig

	// If you want some endpoints to be public but optionally-authenticated, set Optional=true.
	Optional bool
}

type ProviderConfig struct {
	Name     string
	Issuer   string // e.g. https://YOUR_DOMAIN/ (Auth0) or https://cognito-idp.REGION.amazonaws.com/POOL_ID
	Audience string // API audience (Auth0) or client-id / resource server id you validate against
	// If you want to pin acceptable algorithms:
	AllowedAlgs map[string]bool // e.g. {"RS256": true}
	HTTPClient  *http.Client
	CacheTTL    time.Duration // JWKS cache TTL
}

func Middleware(cfg Config) (func(http.Handler) http.Handler, error) {
	if len(cfg.Providers) == 0 {
		return nil, errors.New("authmw: at least one provider is required")
	}

	verifiers := make([]*verifier, 0, len(cfg.Providers))
	for _, p := range cfg.Providers {
		v, err := newVerifier(p)
		if err != nil {
			return nil, err
		}
		verifiers = append(verifiers, v)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			if token == "" {
				if cfg.Optional {
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "missing Authorization: Bearer token", http.StatusUnauthorized)
				return
			}

			// Try each provider (useful for migration / multiple envs)
			var lastErr error
			for _, v := range verifiers {
				u, err := v.verify(r.Context(), token)
				if err == nil {
					ctx := context.WithValue(r.Context(), CtxUser, u)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				lastErr = err
			}

			http.Error(w, fmt.Sprintf("invalid token: %v", lastErr), http.StatusUnauthorized)
		})
	}, nil
}

func FromContext(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(CtxUser).(*User)
	return u, ok
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

/*
Minimal JWT verification:
- Parse header + payload (base64url)
- Verify alg, kid
- Fetch JWKS from issuer's OIDC discovery
- Verify RS256 signature
- Validate iss, aud, exp, nbf

This avoids vendor SDK coupling and works across Auth0/Okta/Cognito.
*/

type verifier struct {
	p ProviderConfig

	discMu sync.Mutex
	disc   *oidcDiscovery

	jwksMu   sync.Mutex
	jwks     *jwksCache
	jwksExp  time.Time
	jwksOnce sync.Once
}

type oidcDiscovery struct {
	Issuer  string `json:"issuer"`
	JWKSURI string `json:"jwks_uri"`
}

type jwksCache struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func newVerifier(p ProviderConfig) (*verifier, error) {
	if p.Issuer == "" || p.Audience == "" {
		return nil, fmt.Errorf("authmw: provider %q requires Issuer and Audience", p.Name)
	}
	if p.HTTPClient == nil {
		p.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	if p.CacheTTL <= 0 {
		p.CacheTTL = 10 * time.Minute
	}
	if p.AllowedAlgs == nil {
		p.AllowedAlgs = map[string]bool{"RS256": true}
	}
	return &verifier{p: p}, nil
}

func (v *verifier) verify(ctx context.Context, token string) (*User, error) {
	hdr, payload, sig, err := splitJWT(token)
	if err != nil {
		return nil, err
	}

	alg, _ := hdr["alg"].(string)
	kid, _ := hdr["kid"].(string)
	if !v.p.AllowedAlgs[alg] {
		return nil, fmt.Errorf("alg not allowed: %s", alg)
	}
	if kid == "" {
		return nil, errors.New("missing kid")
	}

	iss, _ := payload["iss"].(string)
	if normalizeIssuer(iss) != normalizeIssuer(v.p.Issuer) {
		return nil, fmt.Errorf("issuer mismatch: %s", iss)
	}

	if err := validateAudience(payload, v.p.Audience); err != nil {
		return nil, err
	}

	if err := validateTimeClaims(payload, time.Now()); err != nil {
		return nil, err
	}

	pub, err := v.getPublicKey(ctx, kid)
	if err != nil {
		return nil, err
	}

	signingInput := signingInputFromParts(token)
	if err := verifyRS256(signingInput, sig, pub); err != nil {
		return nil, fmt.Errorf("signature invalid: %w", err)
	}

	sub, _ := payload["sub"].(string)
	email, _ := payload["email"].(string)

	return &User{
		Subject: sub,
		Email:   email,
		Issuer:  iss,
		Raw:     payload,
	}, nil
}

func normalizeIssuer(s string) string {
	return strings.TrimRight(s, "/")
}

func splitJWT(token string) (map[string]any, map[string]any, []byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, nil, nil, errors.New("token must have 3 parts")
	}
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, nil, errors.New("bad header b64")
	}
	pb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, nil, errors.New("bad payload b64")
	}
	sb, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, nil, nil, errors.New("bad signature b64")
	}
	var hdr map[string]any
	var payload map[string]any
	if err := json.Unmarshal(hb, &hdr); err != nil {
		return nil, nil, nil, errors.New("bad header json")
	}
	if err := json.Unmarshal(pb, &payload); err != nil {
		return nil, nil, nil, errors.New("bad payload json")
	}
	return hdr, payload, sb, nil
}

func signingInputFromParts(token string) []byte {
	// header.payload (everything before last dot)
	i := strings.LastIndex(token, ".")
	return []byte(token[:i])
}

func validateAudience(payload map[string]any, audExpected string) error {
	audVal, ok := payload["aud"]
	if !ok {
		return errors.New("missing aud")
	}
	switch aud := audVal.(type) {
	case string:
		if aud != audExpected {
			return fmt.Errorf("aud mismatch: %s", aud)
		}
	case []any:
		for _, x := range aud {
			if s, ok := x.(string); ok && s == audExpected {
				return nil
			}
		}
		return fmt.Errorf("aud mismatch: %v", aud)
	default:
		return fmt.Errorf("aud has unexpected type: %T", audVal)
	}
	return nil
}

func validateTimeClaims(payload map[string]any, now time.Time) error {
	// exp required; nbf optional; allow small clock skew if you want
	expF, ok := asFloat(payload["exp"])
	if !ok {
		return errors.New("missing/invalid exp")
	}
	if now.Unix() >= int64(expF) {
		return errors.New("token expired")
	}
	if nbfF, ok := asFloat(payload["nbf"]); ok {
		if now.Unix() < int64(nbfF) {
			return errors.New("token not yet valid (nbf)")
		}
	}
	return nil
}

func asFloat(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

func (v *verifier) discovery(ctx context.Context) (*oidcDiscovery, error) {
	v.discMu.Lock()
	defer v.discMu.Unlock()

	if v.disc != nil {
		return v.disc, nil
	}
	url := normalizeIssuer(v.p.Issuer) + "/.well-known/openid-configuration"
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := v.p.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("discovery failed: %s", resp.Status)
	}
	var d oidcDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, err
	}
	v.disc = &d
	return &d, nil
}

func (v *verifier) getPublicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	// cached JWKS
	v.jwksMu.Lock()
	if v.jwks != nil && time.Now().Before(v.jwksExp) {
		pub, err := keyFromJWKS(v.jwks, kid)
		v.jwksMu.Unlock()
		return pub, err
	}
	v.jwksMu.Unlock()

	// refresh
	d, err := v.discovery(ctx)
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", d.JWKSURI, nil)
	resp, err := v.p.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("jwks fetch failed: %s", resp.Status)
	}
	var jw jwksCache
	if err := json.NewDecoder(resp.Body).Decode(&jw); err != nil {
		return nil, err
	}

	v.jwksMu.Lock()
	v.jwks = &jw
	v.jwksExp = time.Now().Add(v.p.CacheTTL)
	v.jwksMu.Unlock()

	return keyFromJWKS(&jw, kid)
}

func keyFromJWKS(jw *jwksCache, kid string) (*rsa.PublicKey, error) {
	for _, k := range jw.Keys {
		if k.Kid != kid {
			continue
		}
		if k.Kty != "RSA" {
			return nil, fmt.Errorf("unsupported kty: %s", k.Kty)
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, errors.New("bad jwk n")
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, errors.New("bad jwk e")
		}
		eInt := 0
		for _, b := range eBytes {
			eInt = eInt<<8 + int(b)
		}
		pub := &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: eInt,
		}
		return pub, nil
	}
	return nil, errors.New("kid not found in jwks")
}

// verifyRS256 uses crypto/rsa + sha256
func verifyRS256(signingInput []byte, signature []byte, pub *rsa.PublicKey) error {
	// Implemented inline to avoid external deps; feel free to switch to a well-known JWT lib later.
	// RS256 = RSA PKCS1v15 + SHA256
	h := sha256Sum(signingInput)
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, h, signature)
}

func sha256Sum(b []byte) []byte {
	h := sha256.New()
	_, _ = h.Write(b)
	return h.Sum(nil)
}
