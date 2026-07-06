package db

import "testing"

func TestMongoHost(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"mongodb://localhost:27017", "localhost:27017"},
		{"mongodb://user:pass@localhost:27017/plexams", "localhost:27017"},
		{"mongodb://user:pass@db.example.com:27017/plexams?authSource=admin", "db.example.com:27017"},
		{"mongodb+srv://user:secret@cluster0.abcd.mongodb.net/?retryWrites=true", "cluster0.abcd.mongodb.net"},
		{"mongodb://host1:27017,host2:27017/db", "host1:27017,host2:27017"},
		{"localhost:27017", "localhost:27017"},
	}
	for _, tt := range tests {
		got := (&DB{uri: tt.uri}).MongoHost()
		if got != tt.want {
			t.Errorf("MongoHost(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}
