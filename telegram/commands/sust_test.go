package commands

import "testing"

func TestParseSetpoint(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    float64
		wantErr bool
	}{
		{name: "integer", input: "42", want: 42},
		{name: "decimal", input: "42.5", want: 42.5},
		{name: "text", input: "wrong", wantErr: true},
		{name: "nan", input: "NaN", wantErr: true},
		{name: "positive infinity", input: "+Inf", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSetpoint(tt.input)
			if (err != nil) != tt.wantErr || (!tt.wantErr && got != tt.want) {
				t.Fatalf("parseSetpoint(%q) = %v, %v", tt.input, got, err)
			}
		})
	}
}
