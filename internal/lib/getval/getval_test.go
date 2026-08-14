package getval

import (
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aaa2ppp/be"
)

func TestGetenv(t *testing.T) {
	const required = true

	t.Run("String", func(t *testing.T) {
		t.Run("present", func(t *testing.T) {
			t.Setenv("NAME", "Alice")
			gv := New(os.LookupEnv)
			v := gv.String("NAME", required, "default")
			be.Equal(t, v, "Alice")
			be.Err(t, gv.Err(), false)
		})

		t.Run("absent, required", func(t *testing.T) {
			gv := New(os.LookupEnv)
			v := gv.String("MISSING", required, "default")
			be.Equal(t, v, "") // zero value for string
			be.Err(t, gv.Err(), ErrRequired)
			be.True(t, strings.Contains(gv.Err().Error(), "MISSING"))
		})

		t.Run("absent, not required", func(t *testing.T) {
			gv := New(os.LookupEnv)
			v := gv.String("MISSING", false, "default")
			be.Equal(t, v, "default")
			be.Err(t, gv.Err(), false)
		})

		t.Run("empty, required", func(t *testing.T) {
			t.Setenv("EMPTY", "")
			gv := New(os.LookupEnv)
			v := gv.String("EMPTY", required, "default")
			be.Equal(t, v, "")
			be.Err(t, gv.Err(), ErrRequired)
		})
	})

	t.Run("Strings", func(t *testing.T) {
		t.Run("present", func(t *testing.T) {
			t.Setenv("TAGS", "go test config")
			gv := New(os.LookupEnv)
			v := gv.Strings("TAGS", required, []string{"default"})
			be.Equal(t, v, []string{"go", "test", "config"})
			be.Err(t, gv.Err(), false)
		})

		t.Run("absent, not required", func(t *testing.T) {
			gv := New(os.LookupEnv)
			defaultVal := []string{"fallback"}
			v := gv.Strings("MISSING", false, defaultVal)
			be.Equal(t, v, defaultVal)
			be.Err(t, gv.Err(), false)
		})
	})

	t.Run("Int", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			t.Setenv("PORT", "8080")
			gv := New(os.LookupEnv)
			v := gv.Int("PORT", required, 80)
			be.Equal(t, v, 8080)
			be.Err(t, gv.Err(), false)
		})

		t.Run("invalid", func(t *testing.T) {
			t.Setenv("PORT", "not-a-number")
			gv := New(os.LookupEnv)
			v := gv.Int("PORT", required, 80)
			be.Equal(t, v, 0) // zero value
			be.Err(t, gv.Err())
			be.True(t, strings.Contains(gv.Err().Error(), "PORT"))
			be.True(t, strings.Contains(gv.Err().Error(), "not-a-number"))
		})
	})

	t.Run("LogLevel", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			t.Setenv("LOG_LEVEL", "DEBUG")
			gv := New(os.LookupEnv)
			v := gv.LogLevel("LOG_LEVEL", required, slog.LevelInfo)
			be.Equal(t, v, slog.LevelDebug)
			be.Err(t, gv.Err(), false)
		})

		t.Run("invalid", func(t *testing.T) {
			t.Setenv("LOG_LEVEL", "INVALID")
			gv := New(os.LookupEnv)
			v := gv.LogLevel("LOG_LEVEL", required, slog.LevelInfo)
			be.Equal(t, v, slog.Level(0)) // zero value
			be.Err(t, gv.Err())
			be.True(t, strings.Contains(gv.Err().Error(), "LOG_LEVEL"))
			be.True(t, strings.Contains(gv.Err().Error(), "INVALID"))
		})
	})

	t.Run("Bool", func(t *testing.T) {
		t.Run("true values", func(t *testing.T) {
			for _, val := range []string{"true", "True", "TRUE", "yes", "1", "on"} {
				t.Run(val, func(t *testing.T) {
					t.Setenv("FLAG", val)
					gv := New(os.LookupEnv)
					v := gv.Bool("FLAG", required, false)
					be.Equal(t, v, true)
					be.Err(t, gv.Err(), false)
				})
			}
		})

		t.Run("false values", func(t *testing.T) {
			for _, val := range []string{"false", "no", "0", "off"} {
				t.Run(val, func(t *testing.T) {
					t.Setenv("FLAG", val)
					gv := New(os.LookupEnv)
					v := gv.Bool("FLAG", required, true)
					be.Equal(t, v, false)
					be.Err(t, gv.Err(), false)
				})
			}
		})

		t.Run("invalid", func(t *testing.T) {
			t.Setenv("FLAG", "maybe")
			gv := New(os.LookupEnv)
			v := gv.Bool("FLAG", required, false)
			be.Equal(t, v, false) // default on error
			be.Err(t, gv.Err())
			be.True(t, strings.Contains(gv.Err().Error(), "FLAG"))
			be.True(t, strings.Contains(gv.Err().Error(), "maybe"))
		})
	})

	t.Run("Duration", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			t.Setenv("TIMEOUT", "5s")
			gv := New(os.LookupEnv)
			v := gv.Duration("TIMEOUT", required, 1*time.Second)
			be.Equal(t, v, 5*time.Second)
			be.Err(t, gv.Err(), false)
		})

		t.Run("invalid", func(t *testing.T) {
			t.Setenv("TIMEOUT", "five seconds")
			gv := New(os.LookupEnv)
			v := gv.Duration("TIMEOUT", required, 1*time.Second)
			be.Equal(t, v, time.Duration(0))
			be.Err(t, gv.Err())
			be.True(t, strings.Contains(gv.Err().Error(), "TIMEOUT"))
			be.True(t, strings.Contains(gv.Err().Error(), "five seconds"))
		})
	})

	t.Run("URL", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			t.Setenv("API_URL", "https://api.example.com")
			gv := New(os.LookupEnv)
			v := gv.URL("API_URL", required, "http://localhost")
			be.Equal(t, v, "https://api.example.com")
			be.Err(t, gv.Err(), false)
		})

		t.Run("invalid", func(t *testing.T) {
			t.Setenv("API_URL", "://invalid-url")
			gv := New(os.LookupEnv)
			v := gv.URL("API_URL", required, "http://localhost")
			be.Equal(t, v, "")
			be.Err(t, gv.Err())
			be.True(t, strings.Contains(gv.Err().Error(), "API_URL"))
			be.True(t, strings.Contains(gv.Err().Error(), "invalid-url"))
		})
	})

	t.Run("multiple errors", func(t *testing.T) {
		t.Setenv("PORT", "not-a-number")
		t.Setenv("TIMEOUT", "five seconds")
		gv := New(os.LookupEnv)
		gv.Int("PORT", required, 80)
		gv.Duration("TIMEOUT", required, 5*time.Second)
		err := gv.Err()
		be.Err(t, err)
		be.True(t, strings.Contains(err.Error(), "PORT"))
		be.True(t, strings.Contains(err.Error(), "TIMEOUT"))
	})
}
