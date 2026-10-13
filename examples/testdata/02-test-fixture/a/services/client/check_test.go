package client

import "testing"

func TestResolveCheck(t *testing.T) {
	tests := []struct {
		name    string
		subject Subject
		want    Status
	}{
		{
			name:    "complete subject resolves",
			subject: Subject{Name: "Test Subject", Email: "subject@example.com"},
			want:    StatusResolved,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveCheck(tt.subject); got != tt.want {
				t.Errorf("ResolveCheck() = %v, want %v", got, tt.want)
			}
		})
	}
}
