package extend

import (
	"bytes"
	"testing"
)

func TestParseTHSJSONPBody(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{name: "empty", body: "", wantErr: true},
		{name: "whitespace", body: " \n\t", wantErr: true},
		{name: "missing opener", body: `callback{"status":"ok"})`, wantErr: true},
		{name: "missing closer", body: `callback({"status":"ok"}`, wantErr: true},
		{name: "empty payload", body: "callback()", wantErr: true},
		{
			name: "valid with trailing semicolon",
			body: "  callback({\"status\":\"ok\"});\n",
			want: `{"status":"ok"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseTHSJSONPBody([]byte(test.body))
			if test.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(got, []byte(test.want)) {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}
