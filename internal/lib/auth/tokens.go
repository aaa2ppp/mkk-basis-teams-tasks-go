package auth

import (
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"aaa2ppp/teams-tasks/internal/lib/logging"
	"aaa2ppp/teams-tasks/internal/model"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

type Config struct {
	Secret        string
	TokenHeader   string
	TokenLifetime time.Duration
	sharedKey     []byte
}

type tokenClaims struct {
	jwt.Claims
	User model.User
}

func (cfg Config) getSharedKey() ([]byte, error) {
	if cfg.sharedKey == nil {
		sharedKey := make([]byte, hex.DecodedLen(len(cfg.Secret)))
		if n, err := hex.Decode(sharedKey, []byte(cfg.Secret)); err != nil || n < 32 {
			return nil, errors.New("token secret must be at least 32 bytes in hex-string")
		}
		cfg.sharedKey = sharedKey[:32]
	}
	return cfg.sharedKey, nil
}

func (cfg Config) generateToken(user model.User) (string, error) {
	now := time.Now()
	var claims tokenClaims

	sharedKey, err := cfg.getSharedKey()
	if err != nil {
		return "", err
	}

	enc, err := jose.NewEncrypter(jose.A256GCM, jose.Recipient{Algorithm: jose.DIRECT, Key: sharedKey}, nil)
	if err != nil {
		return "", err
	}

	claims.IssuedAt = jwt.NewNumericDate(now)
	claims.Expiry = jwt.NewNumericDate(now.Add(cfg.TokenLifetime))
	claims.User = user

	return jwt.Encrypted(enc).Claims(claims).Serialize()
}

func (cfg Config) httpMiddleware(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr := strings.TrimSpace(r.Header.Get(cfg.TokenHeader))
		if tokenStr == "" {
			http.Error(w, cfg.TokenHeader+" header required", http.StatusUnauthorized)
			return
		}

		sharedKey, err := cfg.getSharedKey()
		if err != nil {
			logger := logging.GetLogger(r.Context())
			logger.Error("getSharedKey", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		token, err := jwt.ParseEncrypted(tokenStr, []jose.KeyAlgorithm{jose.DIRECT}, []jose.ContentEncryption{jose.A256GCM})
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		var claims tokenClaims
		if err := token.Claims(sharedKey, &claims); err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		ctx := contextWithUser(r.Context(), claims.User)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

type Tokens struct {
	cfg Config
}

func New(cfg Config) *Tokens {
	return &Tokens{cfg: cfg}
}

func (t *Tokens) GenerateToken(user model.User) (string, error) {
	return t.cfg.generateToken(user)
}

func (t *Tokens) Middleware(next http.Handler) http.HandlerFunc {
	return t.cfg.httpMiddleware(next)
}
