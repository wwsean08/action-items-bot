package postgres

import "testing"

func TestEscapeLikePattern(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain text is unchanged", input: "buy milk", want: "buy milk"},
		{name: "empty string", input: "", want: ""},
		{name: "percent is escaped", input: "50% off", want: `50\% off`},
		{name: "underscore is escaped", input: "snake_case", want: `snake\_case`},
		{name: "backslash is escaped", input: `back\slash`, want: `back\\slash`},
		{name: "all metacharacters together", input: `_100%\`, want: `\_100\%\\`},
		// The underscore in "action_items" is a LIKE metacharacter and is
		// correctly escaped; only the SQL punctuation (quote, semicolon,
		// comment dashes) is genuinely left alone.
		{name: "sql-ish punctuation is left alone", input: `'; DROP TABLE action_items; --`, want: `'; DROP TABLE action\_items; --`},
		{name: "multibyte text is preserved", input: "café ☕", want: "café ☕"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeLikePattern(tt.input); got != tt.want {
				t.Errorf("escapeLikePattern(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
