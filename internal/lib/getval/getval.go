package getval

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrRequired = errors.New("is required")

type LookupFunc func(key string) (string, bool)

type GetValue struct {
	lookup LookupFunc
	errs   []error
}

func New(lookup LookupFunc) *GetValue {
	return &GetValue{lookup: lookup}
}

func (gv *GetValue) Err() error {
	return errors.Join(gv.errs...)
}

type parseFunc[T any] func(s string) (T, error)

func getValue[T any](lookup LookupFunc, key string, required bool, defaultValue T, parse parseFunc[T]) (T, error) {
	var zero T
	s, ok := lookup(key)
	if !ok || s == "" {
		if required {
			return zero, fmt.Errorf("%s %w", key, ErrRequired)
		}
		return defaultValue, nil
	}
	if v, err := parse(s); err != nil {
		return zero, fmt.Errorf("%s invalid: %w", key, err)
	} else {
		return v, nil
	}
}

func (gv *GetValue) String(key string, required bool, defaultValue string) string {
	v, err := getValue(gv.lookup, key, required, defaultValue, func(s string) (string, error) {
		return s, nil
	})
	if err != nil {
		gv.errs = append(gv.errs, err)
	}
	return v
}

func (gv *GetValue) Strings(key string, required bool, defaultValue []string) []string {
	v, err := getValue(gv.lookup, key, required, defaultValue, func(s string) ([]string, error) {
		return strings.Fields(s), nil
	})
	if err != nil {
		gv.errs = append(gv.errs, err)
	}
	return v
}

func (gv *GetValue) Int(key string, required bool, defaultValue int) int {
	v, err := getValue(gv.lookup, key, required, defaultValue, func(s string) (int, error) {
		return strconv.Atoi(s)
	})
	if err != nil {
		gv.errs = append(gv.errs, err)
	}
	return v
}

func (gv *GetValue) LogLevel(key string, required bool, defaultValue slog.Level) slog.Level {
	v, err := getValue(gv.lookup, key, required, defaultValue, func(s string) (slog.Level, error) {
		var v slog.Level
		err := v.UnmarshalText([]byte(s))
		return v, err
	})
	if err != nil {
		gv.errs = append(gv.errs, err)
	}
	return v
}

func (gv *GetValue) Bool(key string, required bool, defaultValue bool) bool {
	v, err := getValue(gv.lookup, key, required, defaultValue, func(s string) (bool, error) {
		switch strings.ToLower(s) {
		case "true", "yes", "on", "enable", "1":
			return true, nil
		case "false", "no", "off", "disable", "0":
			return false, nil
		default:
			return false, fmt.Errorf("invalid boolean value %q for %q, want: true/false, yes/no, on/off, 1/0", s, key)
		}
	})
	if err != nil {
		gv.errs = append(gv.errs, err)
	}
	return v
}

func (gv *GetValue) Duration(key string, required bool, defaultValue time.Duration) time.Duration {
	v, err := getValue(gv.lookup, key, required, defaultValue, func(s string) (time.Duration, error) {
		return time.ParseDuration(s)
	})
	if err != nil {
		gv.errs = append(gv.errs, err)
	}
	return v
}

func (gv *GetValue) URL(key string, required bool, defaultValue string) string {
	v, err := getValue(gv.lookup, key, required, defaultValue, func(s string) (string, error) {
		if _, err := url.Parse(s); err != nil {
			return "", err
		}
		return s, nil
	})
	if err != nil {
		gv.errs = append(gv.errs, err)
	}
	return v
}

func (gv *GetValue) UUID(key string, required bool, defaultValue uuid.UUID) uuid.UUID {
	v, err := getValue(gv.lookup, key, required, defaultValue, func(s string) (uuid.UUID, error) {
		v, err := uuid.Parse(s)
		if err != nil {
			return uuid.UUID{}, err
		}
		return v, nil
	})
	if err != nil {
		gv.errs = append(gv.errs, err)
	}
	return v
}

// TODO: add Enum
