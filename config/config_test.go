package config

import (
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// testErrCheck looks to see if errContains is a substring of err.Error(). If
// not, this calls t.Fatal(). It also calls t.Fatal() if there was an error, but
// errContains is empty. Returns true if you should continue running the test,
// or false if you should stop the test.
func testErrCheck(t *testing.T, name string, errContains string, err error) bool {
	t.Helper()

	if len(errContains) > 0 {
		if err == nil {
			t.Fatalf("%s error = <nil>, should contain %q", name, errContains)
			return false
		}

		if errStr := err.Error(); !strings.Contains(errStr, errContains) {
			t.Fatalf("%s error = %q, should contain %q", name, errStr, errContains)
			return false
		}

		return false
	}

	if err != nil && len(errContains) == 0 {
		t.Fatalf("%s unexpected error: %v", name, err)
		return false
	}

	return true
}

func cmpDiff(t *testing.T, thing, diff string) {
	t.Helper()

	if len(diff) > 0 {
		t.Fatalf("%s mismatch (-want +got)\n%v", thing, diff)
	}
}

func Test_strToEnv(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want Environment
	}{
		{
			name: "production",
			s:    "production",
			want: Production,
		},
		{
			name: "staging",
			s:    "staging",
			want: Staging,
		},
		{
			name: "testing",
			s:    "testing",
			want: Testing,
		},
		{
			name: "development",
			s:    "development",
			want: Development,
		},
		{
			name: "unknown",
			s:    "unknown",
			want: Development,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strToEnv(tt.s)
			if got != tt.want {
				t.Fatalf("got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadEnv(t *testing.T) {
	tests := []struct {
		name   string
		before func()
		after  func()
		err    string
		want   C
	}{
		{
			name: "all_values",
			before: func() {
				_ = os.Setenv("PORT", "1234")
				_ = os.Setenv("REDIS_URL", "redis://u:1234@redis.example.org:4321")
				_ = os.Setenv("ENV", "testing")
				_ = os.Setenv("HEROKU_APP_ID", "abc123")
				_ = os.Setenv("HEROKU_APP_NAME", "testApp")
				_ = os.Setenv("HEROKU_DYNO_ID", "def890")
			},
			after: func() {
				s := []string{
					"PORT", "REDIS_URL", "ENV",
					"HEROKU_APP_ID", "HEROKU_APP_NAME",
					"HEROKU_DYNO_ID",
				}

				for _, v := range s {
					_ = os.Unsetenv(v)
				}
			},
			want: C{
				Env:  Testing,
				Port: 1234,
				Heroku: H{
					AppID:   "abc123",
					AppName: "testApp",
					DynoID:  "def890",
				},
				Redis: R{
					Addr:     "redis.example.org:4321",
					User:     "u",
					Password: "1234",
				},
			},
		},
		{
			name: "no_password",
			before: func() {
				_ = os.Setenv("PORT", "1234")
				_ = os.Setenv("REDIS_URL", "redis://u@redis.example.org:4321")
				_ = os.Setenv("ENV", "testing")
				_ = os.Setenv("HEROKU_APP_ID", "abc123")
				_ = os.Setenv("HEROKU_APP_NAME", "testApp")
				_ = os.Setenv("HEROKU_DYNO_ID", "def890")
			},
			after: func() {
				s := []string{
					"PORT", "REDIS_URL", "ENV",
					"HEROKU_APP_ID", "HEROKU_APP_NAME",
					"HEROKU_DYNO_ID",
				}

				for _, v := range s {
					_ = os.Unsetenv(v)
				}
			},
			want: C{
				Env:  Testing,
				Port: 1234,
				Heroku: H{
					AppID:   "abc123",
					AppName: "testApp",
					DynoID:  "def890",
				},
				Redis: R{
					Addr: "redis.example.org:4321",
					User: "u",
				},
			},
		},
		{
			name: "bad_REDIS_URL",
			before: func() {
				_ = os.Setenv("PORT", "1234")
				_ = os.Setenv("REDIS_URL", "://")
				_ = os.Setenv("ENV", "testing")
				_ = os.Setenv("HEROKU_APP_ID", "abc123")
				_ = os.Setenv("HEROKU_APP_NAME", "testApp")
				_ = os.Setenv("HEROKU_DYNO_ID", "def890")
			},
			after: func() {
				s := []string{
					"PORT", "REDIS_URL", "ENV",
					"HEROKU_APP_ID", "HEROKU_APP_NAME",
					"HEROKU_DYNO_ID",
				}

				for _, v := range s {
					_ = os.Unsetenv(v)
				}
			},
			err: `failed to parse REDIS_URL: parse "://": missing protocol scheme`,
		},
		{
			name: "bad_PORT",
			before: func() {
				_ = os.Setenv("PORT", "abcxyz")
				_ = os.Setenv("ENV", "testing")
				_ = os.Setenv("HEROKU_APP_ID", "abc123")
				_ = os.Setenv("HEROKU_APP_NAME", "testApp")
				_ = os.Setenv("HEROKU_DYNO_ID", "def890")
			},
			after: func() {
				s := []string{
					"PORT", "REDIS_URL", "ENV",
					"HEROKU_APP_ID", "HEROKU_APP_NAME",
					"HEROKU_DYNO_ID",
				}

				for _, v := range s {
					_ = os.Unsetenv(v)
				}
			},
			err: `failed to parse PORT: strconv.ParseUint: parsing "abcxyz": invalid syntax`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.before != nil {
				tt.before()
			}

			if tt.after != nil {
				defer tt.after()
			}

			got, err := LoadEnv()
			if cont := testErrCheck(t, "LoadEnv()", tt.err, err); !cont {
				return
			}

			cmpDiff(t, "C", cmp.Diff(tt.want, got))
		})
	}
}