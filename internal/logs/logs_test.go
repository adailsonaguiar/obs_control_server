package logs

import "testing"

func TestBufferLimitsAndFiltersEntries(t *testing.T) {
	buffer := New(2)
	buffer.Add("server", "info", "one")
	buffer.Add("obs", "error", "two")
	buffer.Add("server", "info", "three")
	if got := buffer.List("all"); len(got) != 2 || got[0].Message != "two" {
		t.Fatalf("limite inesperado: %+v", got)
	}
	if got := buffer.List("errors"); len(got) != 1 || got[0].Category != "obs" {
		t.Fatalf("filtro inesperado: %+v", got)
	}
}
