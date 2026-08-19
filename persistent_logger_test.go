package semanticrouter

import "testing"

func TestDBLoggerBindsPostgresParameters(t *testing.T) {
	logger := &DBLogger{driver: "postgres"}
	got := logger.bindQuery("VALUES (?, ?, ?)")
	if got != "VALUES ($1, $2, $3)" {
		t.Fatalf("postgres query = %q", got)
	}

	logger.driver = "mysql"
	got = logger.bindQuery("VALUES (?, ?)")
	if got != "VALUES (?, ?)" {
		t.Fatalf("mysql query changed: %q", got)
	}
}
