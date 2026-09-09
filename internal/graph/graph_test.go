package graph

import "testing"

func TestRelID(t *testing.T) {
	const mod = "example.com/mod"
	cases := []struct{ in, want string }{
		{"example.com/mod", "."},
		{"example.com/mod/app/router", "./app/router"},
	}
	for _, tc := range cases {
		if got := relID(mod, tc.in); got != tc.want {
			t.Errorf("relID(%q, %q) = %q, want %q", mod, tc.in, got, tc.want)
		}
	}
}

func TestFullPkg(t *testing.T) {
	const mod = "example.com/mod"
	cases := []struct{ in, want string }{
		{".", "example.com/mod"},
		{"./app/router", "example.com/mod/app/router"},
	}
	for _, tc := range cases {
		if got := fullPkg(mod, tc.in); got != tc.want {
			t.Errorf("fullPkg(%q, %q) = %q, want %q", mod, tc.in, got, tc.want)
		}
	}
}

func TestRelIDFullPkgRoundTrip(t *testing.T) {
	const mod = "example.com/mod"
	for _, imp := range []string{"example.com/mod", "example.com/mod/a", "example.com/mod/a/b/c"} {
		if got := fullPkg(mod, relID(mod, imp)); got != imp {
			t.Errorf("round trip of %q = %q", imp, got)
		}
	}
}

func TestLibLabel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"github.com/gorilla/mux", "gorilla/mux"},
		{"gorm.io/gorm", "gorm.io/gorm"},
	}
	for _, tc := range cases {
		if got := libLabel(tc.in); got != tc.want {
			t.Errorf("libLabel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
